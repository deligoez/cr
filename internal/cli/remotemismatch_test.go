package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// briefIssue writes the issue text `cr brief` reads instead of a tracker, and
// returns its path.
func briefIssue(t *testing.T) string {
	t.Helper()
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(fixtureIssue+": panic once.\n"), 0o600))
	return issue
}

// QA D-V1a-1: `--repo` names a repository none of the clone's GitHub remotes
// points at, and the clone lacks the pull request's head. The refusal is the
// missing commit's, exit 3 with its message, for `cr brief` and `cr status`
// alike, and the hint names both repositories instead of sending the reader to
// a `git fetch` from a remote that is not the pull request's repository. A
// remote on another host names no GitHub repository, so it is not listed even
// when its path reads octocat/hello.
func TestARepoNoRemotePointsAtIsNamedInTheFetchHint(t *testing.T) {
	clone, head := unfetchedClone(t)
	mustGit(t, clone, "remote", "set-url", "origin", "git@github.com:someone/elsewhere.git")
	mustGit(t, clone, "remote", "add", "mirror", "https://gitlab.com/octocat/hello.git")
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	said := "the repository at " + clone + " does not hold commit " + head
	hint := "`--repo` names octocat/hello, and no remote of the repository under review points at it " +
		"(origin points at someone/elsewhere), so a `git fetch` from its remotes may not bring the commit " +
		"the message names; check that `--repo` names the pull request's repository; if it does, fetch " +
		"the commit from it with `git fetch https://github.com/octocat/hello pull/7/head` and run the " +
		"command again, or run the command in a clone of octocat/hello"

	_, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", briefIssue(t))
	var missing *git.MissingCommitError
	require.ErrorAs(t, err, &missing, "cr brief")
	assert.Equal(t, said, err.Error())
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, hint, hintFor(err))
	_, err = layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.Error(t, err, "a brief that could not read the head opens no round")

	recordRoundAt(t, layout, head)
	_, err = runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.ErrorAs(t, err, &missing, "cr status")
	assert.Equal(t, said, err.Error())
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, hint, hintFor(err))
}

// A remote that points at the `--repo` repository, in any spelling git
// accepts for GitHub, keeps the fetch hint: the ssh shorthand and `ssh://`,
// https, with and without `.git`, in another letter case, and beside a second
// remote that points elsewhere, as a fork's clone with an upstream remote does.
func TestARemotePointingAtTheRepoInAnySpellingKeepsTheFetchHint(t *testing.T) {
	for name, remotes := range map[string][][2]string{
		"ssh shorthand":        {{"origin", "git@github.com:octocat/hello.git"}},
		"ssh shorthand bare":   {{"origin", "git@github.com:octocat/hello"}},
		"ssh url":              {{"origin", "ssh://git@github.com/octocat/hello.git"}},
		"https":                {{"origin", "https://github.com/octocat/hello.git"}},
		"https bare":           {{"origin", "https://github.com/octocat/hello"}},
		"letter case":          {{"origin", "https://github.com/OctoCat/Hello.git"}},
		"fork beside upstream": {{"origin", "git@github.com:someone/hello.git"}, {"upstream", "https://github.com/octocat/hello"}},
	} {
		t.Run(name, func(t *testing.T) {
			clone, head := unfetchedClone(t)
			mustGit(t, clone, "remote", "set-url", "origin", remotes[0][1])
			for _, remote := range remotes[1:] {
				mustGit(t, clone, "remote", "add", remote[0], remote[1])
			}
			require.NoError(t, state.New(crHome(t)).Init())

			_, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
				"--issue", fixtureIssue, "--intent-file", briefIssue(t))
			var missing *git.MissingCommitError
			require.ErrorAs(t, err, &missing)
			assert.Equal(t, "the repository at "+clone+" does not hold commit "+head, err.Error())
			assert.Equal(t, ExitFile, exitCodeFor(err))
			assert.Equal(t, fetchHint, hintFor(err))
		})
	}
}

// Nothing is refused for the mismatch alone: a clone of a fork, whose one
// remote points at the fork rather than at the upstream `--repo` names, briefs
// the pull request when it holds the pull request's head.
func TestAForkCloneHoldingTheHeadBriefsTheUpstreamRepo(t *testing.T) {
	clone, head := unfetchedClone(t)
	mustGit(t, clone, "fetch", "--quiet", "origin", fixtureHeadBranch)
	mustGit(t, clone, "cat-file", "-e", head+"^{commit}")
	mustGit(t, clone, "remote", "set-url", "origin", "git@github.com:someone/hello.git")
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())

	_, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", briefIssue(t))
	require.NoError(t, err)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	assert.Equal(t, head, meta.Head)
	assert.Equal(t, 1, meta.Round)
}
