package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trackedFixture builds a repository with one commit, and returns it with the
// revision that commit is at.
func trackedFixture(t *testing.T) (repo, head string) {
	t.Helper()
	repo = fixtureRepo(t)
	writeFixtureFile(t, repo, "app.txt", "the base\n")
	writeFixtureFile(t, repo, "a name with spaces.txt", "also committed\n")
	fixtureGit(t, repo, "add", "app.txt", "a name with spaces.txt")
	fixtureGit(t, repo, "commit", "--quiet", "-m", "the base")
	return repo, fixtureGit(t, repo, "rev-parse", "HEAD")
}

// Head is the revision the worktree is checked out at, which §5.1.6 compares
// against the round's head before every probe or test run.
func TestHeadIsTheRevisionTheWorktreeIsAt(t *testing.T) {
	repo, head := trackedFixture(t)

	at, err := Head(repo)

	require.NoError(t, err)
	assert.Equal(t, head, at)
}

// TrackedState is the deviation of the working tree from HEAD, and the paths
// that deviation is about.
//
// The untracked file is the half worth stating: §5.1.6 measures tracked files
// against the baseline and ignores everything else, because `sandbox.copy` and
// `sandbox.setup` create untracked files by design. A state that counted one
// would report a sandbox as changed for having been prepared.
//
// The path holding spaces is the reason the paths are asked of git rather than
// parsed out of the patch. A `diff --git` header quotes such a name, and a
// report that named the quoted form would name a file that does not exist.
func TestTrackedStateIsTheDeviationFromHead(t *testing.T) {
	repo, _ := trackedFixture(t)

	clean, err := TrackedState(repo)
	require.NoError(t, err)
	assert.Empty(t, clean.Patch, "a checkout as HEAD left it deviates from HEAD in nothing")
	assert.Empty(t, clean.Paths)

	writeFixtureFile(t, repo, "app.txt", "a mutation nobody reverted\n")
	writeFixtureFile(t, repo, "a name with spaces.txt", "changed too\n")
	writeFixtureFile(t, repo, "untracked.txt", "copied in by setup\n")

	dirty, err := TrackedState(repo)
	require.NoError(t, err)

	assert.Equal(t, []string{"a name with spaces.txt", "app.txt"}, dirty.Paths)
	assert.Contains(t, dirty.Patch, "a mutation nobody reverted")
	assert.NotContains(t, dirty.Patch, "untracked.txt",
		"§5.1.6 ignores untracked files, which sandbox.copy and sandbox.setup create by design")
}

// TrackedAmong answers, for the paths §5.1.6's leftover glob matched, which of
// them the repository tracks.
//
// The name holding a `*` is the claim about the pathspecs. They are handed over
// as `:(literal)`, so a file whose own name carries a glob metacharacter is
// matched as itself — without that, git would read the name as a pattern and
// answer about `starA.txt`, reporting an untracked artefact as tracked and
// leaving it in the sandbox for the next run to measure.
func TestTrackedAmongReportsWhichPathsAreTracked(t *testing.T) {
	repo, _ := trackedFixture(t)
	writeFixtureFile(t, repo, "starA.txt", "committed\n")
	fixtureGit(t, repo, "add", "starA.txt")
	fixtureGit(t, repo, "commit", "--quiet", "-m", "a second commit")
	writeFixtureFile(t, repo, "star*.txt", "a leftover probe artefact\n")
	writeFixtureFile(t, repo, "cr_probe_p1.txt", "another one\n")

	tracked, err := TrackedAmong(repo, []string{"app.txt", "cr_probe_p1.txt", "star*.txt"})

	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"app.txt": true}, tracked)
}

// Nothing to ask about is answered without asking. An empty pathspec list would
// otherwise reach git as no pathspec at all, which lists every tracked file in
// the repository and would report each of them as one of the glob's matches.
func TestTrackedAmongAsksNothingOfNoPaths(t *testing.T) {
	repo, _ := trackedFixture(t)

	tracked, err := TrackedAmong(repo, nil)

	require.NoError(t, err)
	assert.Empty(t, tracked)
}
