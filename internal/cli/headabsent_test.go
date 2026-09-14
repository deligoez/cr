package cli

import (
	"encoding/json"
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
