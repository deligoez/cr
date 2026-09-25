package cli

import (
	"bytes"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// populatedPRState runs every command in the tree against a real repository and
// returns the pull request's state directory they left behind.
//
// It is the same fixture TestNoCommandTouchesTheRepositoryUnderReview builds,
// turned the other way round: that guard reads the repository before and after
// and asserts nothing there moved, and this one ignores the repository and
// reads what landed under CR_HOME. Every command is run for the same reason the
// other guard runs them all — the set is taken from the tree, so a command
// added later brings its state here by existing rather than by anyone
// remembering.
func populatedPRState(t *testing.T) string {
	t.Helper()
	fixture := fixtureRepository(t)
	binary := crBinary(t)
	home := t.TempDir()
	root := filepath.Join(home, ".cr")

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	for id, body := range profile.Builtins() {
		require.NoError(t, prepared.EnsureProfile(id, body))
	}
	// A `generic` with a test command that exits 0, so `cr test` and
	// `cr probe run` reach the state they write rather than stopping at a
	// disabled test axis.
	require.NoError(t, os.WriteFile(prepared.Profile("generic"), []byte(
		`{"id":"generic","match":{"files":[],"globs":["**/*"]},`+
			`"axes":{"intent":true,"correctness":true,"convention":true,"test":true},`+
			`"tests":{"cmd":["true"],"globs":["*_test.txt"]}}`), 0o600))
	require.NoError(t, prepared.EnsureRepo(fixtureOwner, fixtureProject))
	require.NoError(t, os.WriteFile(
		prepared.RepoConfig(fixtureOwner, fixtureProject), []byte(`{"profile":"generic"}`), 0o600))
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, Round: 1, Head: fixtureHead,
	}))
	require.NoError(t, held.Write(state.FileUnits,
		[]byte(`{"id":"u1","head":"`+fixtureHead+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	_, err = finding.Waive(prepared, fixtureOwner, fixtureProject, &finding.Waiver{
		WaiverKey: finding.WaiverKey{
			Path: "unrelated.go", Side: "RIGHT", Class: "unchecked-error", ContentHash: "fedcba9876543210",
		},
		Disposition: finding.DispositionNotHere,
	}, finding.WaiverProvenance{Round: 1, PR: fixturePRNumber, Head: fixtureHead})
	require.NoError(t, err)

	// The files the commands are pointed at, all outside the repository
	// under review, as repoRuns requires of them.
	write := func(name, body string) string {
		t.Helper()
		path := filepath.Join(home, name)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		return path
	}
	record := `{"id":"f1","kind":"finding","role":"correctness","class":"unchecked-error",` +
		`"severity":"high","unit":"u1",` +
		`"anchor":{"path":"app.go","side":"RIGHT","start_line":3,"line":3,"content_hash":"0123456789abcdef"},` +
		`"summary":"%s",` +
		`"evidence":"The call's second result is assigned to the blank identifier."}` + "\n"
	merged := write("merged.ndjson", strings.Replace(record, "%s", "Is the returned error dropped?", 1))
	perRole := write("review-correctness.ndjson",
		strings.Replace(record, "%s", "The returned error is dropped.", 1))
	issue := write("issue.txt", "The retry must back off exponentially.\n")
	cells := write("cells.ndjson", `{"unit":"u1","role":"correctness","result":"finding"}`+"\n")
	// §5.7's proposal, with the id §4.6.2 gives the correctness role over the
	// round's one unit; repoRuns says why this file sits outside the
	// repository under review.
	proposals := write("proposals.ndjson", `{"id":"x101","kind":"mutation",`+
		`"role":"correctness","unit":"u1","target":"app.go:3",`+
		`"hypothesis":"No test notices the dropped error.",`+
		`"settles":"A suite that stays green under the mutation proves the gap.",`+
		`"input":"--- a/app.go\n+++ b/app.go\n"}`+"\n")
	pairs := write("mapping.ndjson", "")
	claims := write("claims.ndjson", `{"id":"`+fixtureIssue+`#c1",`+
		`"text":"The retry backs off exponentially.","source":"acceptance",`+
		`"span":"back off exponentially"}`+"\n")
	mutation := write("mutation.diff",
		"--- a/app.go\n+++ b/app.go\n@@ -1,3 +1,3 @@\n package app\n \n"+
			"-func Retry() { backoff() }\n+func Retry() {}\n")

	shims := ghShim(t, t.TempDir(),
		strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch)),
		strings.TrimSpace(mustGit(t, fixture, "rev-parse", "main")))
	env := []string{
		"PATH=" + shims + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + home,
		"TMPDIR=" + t.TempDir(),
		"CR_HOME=" + root,
	}

	runs := repoRuns(merged, claims, issue, cells, proposals, pairs, mutation,
		perRole, filepath.Join(home, "merge-out.ndjson"), write(houseRuleID+".json", houseRuleJSON))
	require.ElementsMatch(t, leafCommands(t), slices.Collect(maps.Keys(runs)),
		"every command in the tree is run against the fixture, so a new one needs an invocation here")
	for _, name := range runOrder(t, runs) {
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(binary, runs[name]...)
		cmd.Dir = fixture
		cmd.Env = env
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		require.NoErrorf(t, cmd.Run(), "cr %s: %s", name, strings.TrimSpace(stderr.String()))
		// §9.5 and §9.6 act on a record that reached GitHub, and no run
		// here posts. The seed and its timing are seedSettledRecords',
		// which explains why straight after `cr post` is the one window
		// it fits in; this fixture needs it for the same reason the
		// §2.2 guard does, and verdicts.ndjson is a §2.3 row only a
		// `cr verify` that ran writes.
		if name == "post" {
			seedSettledRecords(t, prepared)
		}
	}
	return prepared.PRDir(fixtureOwner, fixtureProject, fixturePRNumber)
}

// A pull request's state directory, after a run of every command, holds exactly
// the files §2.3's table names — and holds every one of them.
//
// Both directions are the point. The forward one says the table is not a wish:
// each row is a file a real run produces, so a row nothing writes would fail
// here instead of reading as documentation. The backward one says the table is
// complete: state cr keeps under a name no row gives is state a reader cannot
// find, back it up, or reason about, and §2.3 is the only place that says what
// a pull request's directory contains.
//
// Two directories sit beside those files and have no row, both because §2.3's
// table is a table of state files and neither holds one. `cr review` creates
// `fanout/<n>/<unit>/` and writes nothing into it, since §4.6.2's output files
// are the roles' to write and `cr merge`'s to read — asserted below rather
// than assumed. `cr sandbox create` checks §5.1.1's worktree out at
// `sandbox/`, and what is in it is the repository under review at the round's
// head plus §5.1.2's copies and §5.1.3's setup, none of it cr's own state.
//
// They are named from state.DirFanOut and state.DirSandbox rather than spelled
// here, and they are the only two admitted: a third directory appearing under
// a pull request's state fails this test, which is the signal that §2.3 owes
// the reader a row or a sentence about it.
func TestAPopulatedStateDirectoryIsExactlySection23sTable(t *testing.T) {
	dir := populatedPRState(t)
	// The two trees §2.3's table does not describe, each with the section
	// that does.
	untabled := map[string]string{
		state.DirFanOut:  "§4.6.2",
		state.DirSandbox: "§5.1.1",
	}

	flat := state.PRNamed()
	perRound := state.RoundNamed()
	found := make([]string, 0, len(flat)+len(perRound))
	require.NoError(t, filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		parts := strings.Split(rel, "/")
		switch {
		case rel == ".":
			return nil
		case parts[0] == state.DirFanOut:
			assert.Truef(t, entry.IsDir(),
				"§4.6.2's output files are the agent's, and cr wrote %s", rel)
			return nil
		case untabled[parts[0]] != "":
			// The worktree's own contents, which belong to the
			// section that puts it here and not to §2.3.
			return fs.SkipDir
		case entry.IsDir():
			assert.Truef(t, rel == "rounds" || (len(parts) == 2 && parts[0] == "rounds"),
				"%s is a directory §2.3 gives a pull request no row for", rel)
			return nil
		case len(parts) == 3 && parts[0] == "rounds":
			assert.Containsf(t, perRound, parts[2], "§2.3 gives a round no %s", parts[2])
			found = append(found, "rounds/<n>/"+parts[2])
		default:
			assert.Lenf(t, parts, 1, "%s sits where §2.3 puts no file", rel)
			assert.Containsf(t, flat, rel, "§2.3 gives a pull request no %s", rel)
			found = append(found, rel)
		}
		return nil
	}))

	want := flat
	for _, name := range perRound {
		want = append(want, "rounds/<n>/"+name)
	}
	assert.ElementsMatch(t, want, found,
		"§2.3's table and what a run of every command leaves behind are the same set of files")
}
