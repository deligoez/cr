package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// fetchHint is the step a head the clone does not hold is met with.
const fetchHint = "run `git fetch` in the repository under review so it holds the commit the message names " +
	"(`git fetch origin pull/<pr>/head` for a pull request from a fork), then run the command again"

// forcePushedClone is QA W3's push-03: a round recorded at a head that a
// force-push then took off the head branch, read from a fresh clone made after
// the push, which therefore never received the recorded head.
//
// The origin keeps the old commit as an unreachable object, so the absence is
// the clone's: `git clone --no-local` transfers only what a ref reaches. It
// returns the state layout, the recorded head, and the head the push left.
func forcePushedClone(t *testing.T) (layout state.Layout, recorded, moved string) {
	t.Helper()
	origin := t.TempDir()
	write := func(body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(origin, "lib.go"), []byte(body), 0o600))
	}
	mustGit(t, origin, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("package lib\n\nfunc Load() {}\n")
	mustGit(t, origin, "add", "lib.go")
	mustGit(t, origin, "commit", "--quiet", "-m", "the base")
	mustGit(t, origin, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("package lib\n\nfunc Load() {\n\tpanic(\"one\")\n}\n")
	mustGit(t, origin, "commit", "--quiet", "-am", "the change the round was opened on")
	recorded = strings.TrimSpace(mustGit(t, origin, "rev-parse", fixtureHeadBranch))
	mustGit(t, origin, "checkout", "--quiet", "-B", fixtureHeadBranch, "main")
	write("package lib\n\nfunc Load() {\n\tpanic(\"two\")\n}\n")
	mustGit(t, origin, "commit", "--quiet", "-am", "the history the force-push left")
	moved = strings.TrimSpace(mustGit(t, origin, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, origin, "rev-parse", "main"))

	clone := filepath.Join(t.TempDir(), "fresh")
	mustGit(t, t.TempDir(), "clone", "--quiet", "--no-local", origin, clone)
	_, err := gitIn(t, clone, "cat-file", "-e", recorded+"^{commit}")
	require.Error(t, err, "the fixture is only worth anything if the clone lacks the recorded head")
	mustGit(t, clone, "cat-file", "-e", moved+"^{commit}")

	restore := repoDir
	repoDir = func() (string, error) { return clone, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), moved, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout = state.New(crHome(t))
	require.NoError(t, layout.Init())
	movedHead(t, moved)
	recordRoundAt(t, layout, recorded)
	return layout, recorded, moved
}

// recordRoundAt writes round 1 of the fixture pull request at head, with one
// unit, the way `cr brief` would have left it.
func recordRoundAt(t *testing.T, layout state.Layout, head string) {
	t.Helper()
	require.NoError(t, layout.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, Round: 1, Head: head,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":3,"end":5}],`+
			`"head":"`+head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
}

// §9.3.1 over a clone that never held the recorded head: `cr status` reports
// both heads and names `cr brief`, and reads nothing at the recorded head, so
// the report is not lost to git's refusal of a tree the clone lacks (QA
// D-W3-1, where it exited 3 with `ls-tree … not a tree object`).
//
// What the report leaves out is said rather than zeroed: the file counts are
// absent from the document, not printed as nothing excluded, and the honesty
// channel names the halves and the counts it could not read.
func TestStatusReportsAMovedHeadTheCloneNeverHeld(t *testing.T) {
	_, recorded, moved := forcePushedClone(t)

	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err, "§9.3.1 reports the move, and §9.3.2 refuses only writers")

	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(printed), &document))
	assert.NotContains(t, document, "files", "the counts are read at the recorded head, so none is claimed")
	var report struct {
		Completeness struct {
			Complete bool     `json:"complete"`
			Reasons  []string `json:"reasons"`
		} `json:"completeness"`
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	assert.False(t, report.Completeness.Complete)
	require.NotEmpty(t, report.Completeness.Reasons)
	assert.Equal(t, "§10.2.1: the pull request's head is no longer the head this round was recorded against",
		report.Completeness.Reasons[0])
	require.GreaterOrEqual(t, len(report.Honesty), 2)
	assert.Equal(t, "§9.3.1: round 1 was opened at head "+recorded+" and the pull request's current head is "+
		moved+"; run `cr brief "+fixturePR+" --repo "+fixtureSlug+"` to open the round the current head belongs to",
		report.Honesty[0])
	assert.Equal(t, "§9.3.1: the reinvention half of §4.3.1, the symbol half of §4.4.1, and the file counts of "+
		"§3.4.2 and §3.4.7 are not reported for round 1, because each is read at head "+recorded+
		", which is no longer the pull request's head", report.Honesty[1])
}

// unfetchedClone is QA W3's setup note: a clone made before the head was
// pushed, so GitHub reports a head the clone has never fetched. It returns the
// clone and that head.
func unfetchedClone(t *testing.T) (clone, head string) {
	t.Helper()
	origin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(origin, "lib.go"), []byte("package lib\n\nfunc Load() {}\n"), 0o600))
	mustGit(t, origin, "-c", "init.defaultBranch=main", "init", "--quiet")
	mustGit(t, origin, "add", "lib.go")
	mustGit(t, origin, "commit", "--quiet", "-m", "the base")
	clone = filepath.Join(t.TempDir(), "early")
	mustGit(t, t.TempDir(), "clone", "--quiet", "--no-local", origin, clone)
	mustGit(t, origin, "checkout", "--quiet", "-b", fixtureHeadBranch)
	require.NoError(t, os.WriteFile(filepath.Join(origin, "lib.go"),
		[]byte("package lib\n\nfunc Load() {\n\tpanic(\"one\")\n}\n"), 0o600))
	mustGit(t, origin, "commit", "--quiet", "-am", "the head pushed after the clone")
	head = strings.TrimSpace(mustGit(t, origin, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, origin, "rev-parse", "main"))
	_, err := gitIn(t, clone, "cat-file", "-e", head+"^{commit}")
	require.Error(t, err, "the fixture is only worth anything if the clone lacks the head")

	restore := repoDir
	repoDir = func() (string, error) { return clone, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return clone, head
}

// A head the clone never fetched is a fetch, for `cr brief` and for `cr status`
// alike: exit 3 naming the commit and the repository, and a hint naming
// `git fetch`, rather than git's refusal of a merge base or a tree with a hint
// to run git by hand. `cr brief` records nothing on the way.
func TestAHeadTheCloneNeverFetchedIsAFetch(t *testing.T) {
	clone, head := unfetchedClone(t)
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(fixtureIssue+": panic once.\n"), 0o600))
	said := "the repository at " + clone + " does not hold commit " + head

	_, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	var missing *git.MissingCommitError
	require.ErrorAs(t, err, &missing, "cr brief")
	assert.Equal(t, said, err.Error())
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, fetchHint, hintFor(err))
	_, err = layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.Error(t, err, "a brief that could not read the head opens no round")

	recordRoundAt(t, layout, head)
	_, err = runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.ErrorAs(t, err, &missing, "cr status")
	assert.Equal(t, said, err.Error())
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, fetchHint, hintFor(err))
}

// gitHint is the step a git failure of any other kind is met with.
const gitHint = "the git command the message names failed; run it yourself to see what it reports"

// headAbsentCommands are the four commands headNotFetched classifies for, each
// with an argv that reaches a git read of the pull request's head and base on
// threeUnitHome's drafted round. `cr review` names the intent pass, which is the
// one a round without a mapping may emit.
func headAbsentCommands(t *testing.T) map[string][]string {
	t.Helper()
	return map[string][]string{
		"brief": {"brief", fixturePR, "--repo", fixtureSlug,
			"--issue", fixtureIssue, "--intent-file", briefIssue(t)},
		"status": {"status", fixturePR, "--repo", fixtureSlug},
		"review": {"review", fixturePR, "--repo", fixtureSlug, "--axis", "intent"},
		"post":   {"post", fixturePR, "--repo", fixtureSlug},
	}
}

// draftedCloneLacking is threeUnitHome's round with one queued record drafted,
// read from a no-local clone of its repository that lacks one commit GitHub
// reports: the pull request's head, or a base that moved after the clone was
// made. It returns the clone and the commit it lacks.
func draftedCloneLacking(t *testing.T, lacking string) (clone, missing string) {
	t.Helper()
	unitDrafted(t, onUnit("u1", "money.go", "RIGHT", 12, 12))
	full, err := repoDir()
	require.NoError(t, err)
	head := strings.TrimSpace(mustGit(t, full, "rev-parse", fixtureHeadBranch))
	branch := "main"
	missing = head
	if lacking == "base" {
		branch = fixtureHeadBranch
		tree := strings.TrimSpace(mustGit(t, full, "rev-parse", "main^{tree}"))
		missing = strings.TrimSpace(mustGit(t, full, "commit-tree", "-p", "main", "-m", "the base moved on", tree))
		mustGit(t, full, "update-ref", "refs/heads/main", missing)
		t.Setenv("PATH", ghShim(t, t.TempDir(), head, missing)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	clone = filepath.Join(t.TempDir(), "clone")
	mustGit(t, t.TempDir(), "clone", "--quiet", "--no-local", "--single-branch", "--branch", branch, full, clone)
	_, err = gitIn(t, clone, "cat-file", "-e", missing+"^{commit}")
	require.Error(t, err, "the fixture is only worth anything if the clone lacks the commit")

	restore := repoDir
	repoDir = func() (string, error) { return clone, nil }
	t.Cleanup(func() { repoDir = restore })
	return clone, missing
}

// One predicate for four commands: `cr review` and `cr post` on a clone that
// lacks the pull request's head, or its base, give the same missing commit, code
// and `git fetch` hint as `cr brief` and `cr status`, rather than git's refusal
// of a merge base with a hint to run git by hand.
func TestEveryCommandReadingTheHeadGivesTheFetchHint(t *testing.T) {
	for _, lacking := range []string{"head", "base"} {
		for name := range headAbsentCommands(t) {
			t.Run(lacking+"/"+name, func(t *testing.T) {
				clone, absent := draftedCloneLacking(t, lacking)

				_, err := runCLIPrinting(t, headAbsentCommands(t)[name]...)
				var missing *git.MissingCommitError
				require.ErrorAs(t, err, &missing)
				assert.Equal(t, git.MissingCommitError{Dir: clone, Commit: absent}, *missing)
				assert.Equal(t, "the repository at "+clone+" does not hold commit "+absent, err.Error())
				assert.Equal(t, ExitFile, exitCodeFor(err))
				assert.Equal(t, fetchHint, hintFor(err))
			})
		}
	}
}

// The same four commands name a `--repo` no remote of the clone points at, in
// the hint of the missing commit they now classify alike.
func TestEveryCommandReadingTheHeadNamesARepoNoRemotePointsAt(t *testing.T) {
	hint := "`--repo` names octocat/hello, and no remote of the repository under review points at it " +
		"(origin points at someone/elsewhere), so a `git fetch` from its remotes may not bring the commit " +
		"the message names; check that `--repo` names the pull request's repository; if it does, fetch " +
		"the commit from it with `git fetch https://github.com/octocat/hello pull/7/head` and run the " +
		"command again, or run the command in a clone of octocat/hello"
	for name := range headAbsentCommands(t) {
		t.Run(name, func(t *testing.T) {
			clone, absent := draftedCloneLacking(t, "head")
			mustGit(t, clone, "remote", "set-url", "origin", "git@github.com:someone/elsewhere.git")

			_, err := runCLIPrinting(t, headAbsentCommands(t)[name]...)
			var mismatch *RemoteMismatchError
			require.ErrorAs(t, err, &mismatch)
			assert.Equal(t, "the repository at "+clone+" does not hold commit "+absent, err.Error())
			assert.Equal(t, ExitFile, exitCodeFor(err))
			assert.Equal(t, hint, hintFor(err))
		})
	}
}

// The other direction: a git failure while the clone holds both the head and
// the base is not a fetch. The head commit's tree is taken out of the object
// store, so every read of the head's content fails, and each of the four
// commands keeps git's own error and hint.
func TestAGitFailureOfAnotherKindKeepsItsOwnHint(t *testing.T) {
	for name := range headAbsentCommands(t) {
		t.Run(name, func(t *testing.T) {
			unitDrafted(t, onUnit("u1", "money.go", "RIGHT", 12, 12))
			full, err := repoDir()
			require.NoError(t, err)
			tree := strings.TrimSpace(mustGit(t, full, "rev-parse", fixtureHeadBranch+"^{tree}"))
			require.NoError(t, os.Remove(filepath.Join(full, ".git", "objects", tree[:2], tree[2:])))
			mustGit(t, full, "cat-file", "-e", fixtureHeadBranch+"^{commit}")
			mustGit(t, full, "cat-file", "-e", "main^{commit}")

			_, err = runCLIPrinting(t, headAbsentCommands(t)[name]...)
			var failed *git.CommandError
			require.ErrorAs(t, err, &failed)
			var missing *git.MissingCommitError
			assert.False(t, errors.As(err, &missing), "the clone holds both commits")
			assert.Equal(t, failed.Error(), err.Error())
			assert.Equal(t, ExitFile, exitCodeFor(err))
			assert.Equal(t, gitHint, hintFor(err))
		})
	}
}
