package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §5.2.1 through the command: `cr test` runs the profile's test command inside
// the sandbox, narrowed by `tests.filter_flag`, and reports §5.1.6's recreation.
//
// No sandbox is created beforehand, and that is deliberate. §5.1.6 has cr verify
// the sandbox before every run and recreate one that fails the check, and an
// absent sandbox fails it — so this exercises the check, the recreation, and the
// notice in one go. The notice is asserted in the payload because §11.1 exempts
// it from `--quiet`: a sandbox rebuilt in silence is a run whose previous probe
// left something behind, read by the author as if nothing had happened.
//
// The runner writes its own working directory and its own arguments to a log
// outside the sandbox, so what is checked is where the command stood and what it
// was given, rather than what the payload says about itself.
func TestTheTestCommandRunsTheProfileRunnerInTheSandbox(t *testing.T) {
	fixture := fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	scripts := t.TempDir()
	log := filepath.Join(scripts, "observed.log")
	runner := filepath.Join(scripts, "runner.sh")
	require.NoError(t, os.WriteFile(runner,
		[]byte("#!/bin/sh\n{ pwd; echo \"$@\"; } > "+log+"\n"), 0o700))

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"tests":{"cmd":["`+runner+`"],"globs":["*_test.txt"],"filter_flag":"--only"}}`))
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, ProfileID: "qa", Round: 1, Head: head,
	}))
	require.NoError(t, held.Unlock())

	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })

	var printed map[string]any
	require.NoError(t, json.Unmarshal([]byte(throughAPipe(t,
		"test", fixturePR, "--repo", fixtureSlug, "--filter", "retries twice")), &printed))

	sandboxPath := prepared.Sandbox(fixtureOwner, fixtureProject, fixturePRNumber)
	assert.Equal(t, sandboxPath, printed["sandbox"])
	assert.Equal(t, []any{runner, "--only", "retries twice"}, printed["command"],
		"§5.2.1: --filter is passed as the profile's tests.filter_flag")
	assert.Equal(t, "retries twice", printed["filter"])
	assert.InDelta(t, 0, printed["exit_code"], 0)

	honesty, ok := printed["honesty"].([]any)
	require.True(t, ok, "§11.1's disclosures are a field on the payload")
	require.Len(t, honesty, 1, "§5.1.6: the sandbox was absent, so it was recreated and said so")
	assert.Contains(t, honesty[0], "§5.1.6")
	assert.Contains(t, honesty[0], sandboxPath)

	observed, err := os.ReadFile(log)
	require.NoError(t, err, "the profile's test command never ran")
	// The sandbox path is resolved through its symlinks, because a
	// temporary directory reaches `pwd` as the path the kernel resolved.
	resolved, err := filepath.EvalSymlinks(sandboxPath)
	require.NoError(t, err)
	ran := strings.Split(strings.TrimSpace(string(observed)), "\n")
	require.Len(t, ran, 2)
	assert.Equal(t, resolved, ran[0], "§5.2.1: the suite runs inside the sandbox")
	assert.Equal(t, "--only retries twice", ran[1])
}

// §5.2.4 through the command: every run is stored in runs.ndjson under an id
// of the form r<n>, and the id space is what a probe's `baseline` field
// references — so the command prints the id it allocated and the second run of
// the same pull request gets the next one rather than overwriting the first.
//
// The runner exits non-zero and prints more than the profile retains, which is
// what makes the record worth asserting on: the exit code reaches the file
// unread, `timed_out` says the run finished rather than being killed, the tail
// is the end of the output and not the beginning, and the two counts are
// absent because no `tests.count_pattern` is configured to derive them.
func TestTheTestCommandStoresARunRecordForEveryRun(t *testing.T) {
	fixture := fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	printed := "line one\nline two\nTests:  4 passed\n"
	runner := filepath.Join(t.TempDir(), "runner.sh")
	require.NoError(t, os.WriteFile(runner, []byte(
		"#!/bin/sh\necho 'line one'\necho 'line two' >&2\necho 'Tests:  4 passed'\nexit 3\n"), 0o700))

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"tests":{"cmd":["`+runner+`"],"globs":["*_test.txt"],"output_tail_bytes":16}}`))
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, ProfileID: "qa", Round: 3, Head: head,
	}))
	require.NoError(t, held.Unlock())

	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })

	for i, want := range []string{"r1", "r2"} {
		// The runner's own output reaches the reader as it is
		// produced, ahead of the document §12.1 keeps stdout for, so
		// the two are separated here rather than the runner being
		// silenced: what is stored has to be what was shown.
		shown := throughAPipe(t, "test", fixturePR, "--repo", fixtureSlug)
		require.True(t, strings.HasPrefix(shown, printed),
			"run %d: the runner's output reaches the reader as it is produced", i+1)

		var reported map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(shown, printed)), &reported))
		assert.Equal(t, want, reported["run"], "run %d reports the id it allocated", i+1)
	}

	body, err := os.ReadFile(prepared.PRFile(
		fixtureOwner, fixtureProject, fixturePRNumber, state.FileRuns))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	require.Len(t, lines, 2, "§5.2.4: a run is added to runs.ndjson, never written over it")

	var stored map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &stored))
	duration, ok := stored["duration_ms"].(float64)
	require.True(t, ok, "§5.2.4 stores the duration")
	assert.GreaterOrEqual(t, duration, float64(0))
	delete(stored, "duration_ms")
	assert.Equal(t, map[string]any{
		"id":          "r1",
		"head":        head,
		"round":       float64(3),
		"exit_code":   float64(3),
		"timed_out":   false,
		"output_tail": printed[len(printed)-16:],
		"passed":      false,
	}, stored,
		"§5.2.4: no filter was given, no count is derivable, and the run was not a probe's")
}

// §5.6.3: cr warns that an unrelated local test run can still collide, because
// the lock only covers cr's own runs.
//
// It is a warning and not an enforcement, and it could not be anything else — a
// `pest` the developer started in another terminal takes no lock of cr's and cr
// cannot see it. What the sentence is against is a held lock reading as a
// guarantee: a suite that failed because two runs shared a database is exactly
// the failure someone would otherwise attribute to the code under review.
//
// Both renderings are checked, because they fail apart: an agent reads the
// document and a person reads the terminal, and a warning that reached only one
// of them has not been given to the reader who was about to act on it.
func TestTheTestCommandWarnsThatTheProbeLockCoversOnlyItsOwnRuns(t *testing.T) {
	fixture := fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"tests":{"cmd":["true"],"globs":["*_test.txt"]}}`))
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, ProfileID: "qa", Round: 1, Head: head,
	}))
	require.NoError(t, held.Unlock())

	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })

	var reported map[string]any
	require.NoError(t, json.Unmarshal([]byte(throughAPipe(t,
		"test", fixturePR, "--repo", fixtureSlug)), &reported))

	warnings, ok := reported["warnings"].([]any)
	require.True(t, ok, "§5.6.3's warning is a field on the payload")
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "§5.6.3")
	assert.Contains(t, warnings[0], fixture,
		"the warning names the repository whose runs the lock does not cover")

	// The terminal rendering carries the same sentence, and the payload is
	// rendered directly so the check is on the writer rather than on a
	// second invocation's incidental state.
	var printed bytes.Buffer
	out := &writer{out: &printed, mode: ModeText}
	require.NoError(t, out.emit(&testRunResult{
		Run: "r1", Sandbox: "/tmp/sandbox", Command: []string{"true"},
		Warnings: []string{"the probe lock covers cr's own runs only, per §5.6.3"},
		Honesty:  []string{},
	}))
	assert.Contains(t, printed.String(), "§5.6.3")
}

// The terminal rendering says what the run was narrowed to, and says "none"
// when it was not narrowed at all.
//
// An unfiltered run is the baseline §5.2.2 records once per head, and §5.3.6
// turns on which tests a filtered run selected — so the difference between the
// whole suite and a subset is the difference between a result that can support
// a `probed` grade and one that cannot. Printed as a blank it reads as a value
// the command failed to fill in, which is the one reading that is wrong in both
// directions. gremlins found this: the branch was rendered by no test at all.
func TestTheTestRenderingNamesTheFilterOrSaysThereWasNone(t *testing.T) {
	render := func(t *testing.T, filter string) string {
		t.Helper()
		var printed bytes.Buffer
		out := &writer{out: &printed, mode: ModeText}
		require.NoError(t, out.emit(&testRunResult{
			Run: "r1", Sandbox: "/tmp/sandbox", Command: []string{"pest"},
			Filter: filter, Warnings: []string{}, Honesty: []string{},
		}))
		return printed.String()
	}

	assert.Contains(t, render(t, ""), "filter  none",
		"an absent filter reads as an answer rather than as a blank")
	assert.Contains(t, render(t, "retries twice"), "filter  retries twice")
	assert.NotContains(t, render(t, "retries twice"), "none")
}

// §5.2.1's extraction through the command: the counts are summed over every
// match of each pattern, taken from the runner's stdout and stderr merged in
// the order they were written, and read from the whole of that stream rather
// than from the tail the record retains.
//
// The runner prints one status to each stream and the profile retains sixteen
// bytes, so the three assertions separate. A reader of one stream sees 2 or 3
// and never 5. A reader of the tail sees the last line alone, so it too sees 3
// — which is the failure mode that matters, because a truncated view produces
// a wrong count rather than an undetermined one, and §5.2.5 would hand it on
// as a baseline. And nothing in the output says the word failed, which is the
// all-passing case §5.2.1 makes zero rather than undetermined: Pest prints no
// status whose count is zero.
func TestTheStoredCountsAreSummedOverTheWholeMergedStream(t *testing.T) {
	fixture := fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	runner := filepath.Join(t.TempDir(), "runner.sh")
	require.NoError(t, os.WriteFile(runner, []byte(
		"#!/bin/sh\necho 'Tests:  2 passed'\necho 'Tests:  3 passed' >&2\n"), 0o700))

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"tests":{"cmd":["`+runner+`"],"globs":["*_test.txt"],"output_tail_bytes":16,`+
		`"count_pattern":"Tests:\\s+(\\d+)\\s+(?:failed|passed)\\b",`+
		`"failed_pattern":"Tests:\\s+(\\d+)\\s+failed\\b"}}`))
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, ProfileID: "qa", Round: 1, Head: head,
	}))
	require.NoError(t, held.Unlock())

	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })

	throughAPipe(t, "test", fixturePR, "--repo", fixtureSlug)

	body, err := os.ReadFile(prepared.PRFile(
		fixtureOwner, fixtureProject, fixturePRNumber, state.FileRuns))
	require.NoError(t, err)
	var stored map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(string(body))), &stored))

	assert.Equal(t, float64(5), stored["tests_run"],
		"§5.2.1: both matches are summed, from both streams and from beyond the tail")
	assert.Equal(t, float64(0), stored["tests_failed"],
		"§5.2.1: a failed pattern that never matches is zero, not undetermined")
	assert.Equal(t, "ests:  3 passed\n", stored["output_tail"],
		"the tail is still the bounded one, so the count came from elsewhere")
}
