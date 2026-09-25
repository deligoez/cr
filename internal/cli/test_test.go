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
	require.NoError(t, json.Unmarshal([]byte(afterHeader(t, throughAPipe(t,
		"test", fixturePR, "--repo", fixtureSlug, "--filter", "retries twice"))), &printed))

	sandboxPath := prepared.Sandbox(fixtureOwner, fixtureProject, fixturePRNumber)
	assert.Equal(t, sandboxPath, printed["sandbox"])
	assert.Equal(t, []any{runner, "--only", "retries twice"}, printed["command"],
		"§5.2.1: --filter is passed as the profile's tests.filter_flag")
	assert.Equal(t, "retries twice", printed["filter"])
	assert.InDelta(t, 0, printed["exit_code"], 0)

	honesty, ok := printed["honesty"].([]any)
	require.True(t, ok, "§11.1's disclosures are a field on the payload")
	require.Len(t, honesty, 2, "§5.1.6: the sandbox was absent, so it was recreated and said so")
	assert.Contains(t, honesty[0], "§5.1.6")
	assert.Contains(t, honesty[0], sandboxPath)
	assert.Equal(t, "the runner exited 0, but the profile sets no tests.count_pattern, so the executed count "+
		"is undetermined and the run cannot pass or serve as a baseline (§5.2.1, §5.2.5)", honesty[1],
		"§5.2.1: the run exited 0 and no count could be read")

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
		shown := afterHeader(t, throughAPipe(t, "test", fixturePR, "--repo", fixtureSlug))
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
	baseline, err := os.ReadFile(prepared.PRFile(
		fixtureOwner, fixtureProject, fixturePRNumber, state.FileSandboxBaseline))
	require.NoError(t, err)
	var sandboxBaseline map[string]any
	require.NoError(t, json.Unmarshal(baseline, &sandboxBaseline))
	require.NotEmpty(t, sandboxBaseline["generation"], "the sandbox's post-setup baseline names its generation")
	assert.Equal(t, map[string]any{
		"id":           "r1",
		"head":         head,
		"round":        float64(3),
		"exit_code":    float64(3),
		"timed_out":    false,
		"contaminated": false,
		"output_tail":  printed[len(printed)-16:],
		"passed":       false,
		"sandbox":      sandboxBaseline["generation"],
	}, stored,
		"§5.2.4: no filter was given, no count is derivable, and the run was not a probe's; "+
			"the run is stamped with the sandbox it measured")
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
	require.NoError(t, json.Unmarshal([]byte(afterHeader(t, throughAPipe(t,
		"test", fixturePR, "--repo", fixtureSlug))), &reported))

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

// The terminal rendering says what the run was narrowed to — the filter and
// the paths — and says "none" for either when it was not narrowed that way.
//
// An unnarrowed run measures the whole suite, which §5.2.2 makes the baseline
// of an unnarrowed probe, and §5.3.6 turns on which tests a narrowed run
// selected — so the difference between the whole suite and a subset is the
// difference between a result that can support a `probed` grade and one that
// cannot. Printed as a blank it reads as a value the command failed to fill in,
// which is the one reading that is wrong in both directions. gremlins found
// this: the branch was rendered by no test at all.
func TestTheTestRenderingNamesTheFilterOrSaysThereWasNone(t *testing.T) {
	render := func(t *testing.T, filter string, paths []string) string {
		t.Helper()
		var printed bytes.Buffer
		out := &writer{out: &printed, mode: ModeText}
		require.NoError(t, out.emit(&testRunResult{
			Run: "r1", Sandbox: "/tmp/sandbox", Command: []string{"pest"},
			Filter: filter, Paths: paths, Warnings: []string{}, Honesty: []string{},
		}))
		return printed.String()
	}

	assert.Contains(t, render(t, "", nil), "filter  none",
		"an absent filter reads as an answer rather than as a blank")
	assert.Contains(t, render(t, "", nil), "paths   none",
		"and so does an absent path")
	assert.Contains(t, render(t, "retries twice", nil), "filter  retries twice")
	assert.Contains(t, render(t, "", []string{"tests/Unit", "tests/Feature"}),
		"paths   tests/Unit, tests/Feature",
		"§5.2.1 keeps the paths in the order they were given")
	assert.NotContains(t, render(t, "retries twice", []string{"tests/Unit"}), "none",
		"a run narrowed both ways reports neither as absent")
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
	// §5.2.5 through the command, on the only shape that can satisfy it:
	// the runner exited 0, five tests ran and none failed. This is the run
	// §5.2.6 may offer as a baseline, and the verdict is computed at the
	// write rather than supplied by the caller.
	assert.Equal(t, true, stored["passed"],
		"§5.2.5: exit 0, a derivable executed count above zero, and no failures")
}

// The counts reach the command's own output, not only runs.ndjson. Measured
// 2026-09-22 against deligoez/cr-qa-go: v0.6.0's `cr test --json` carried
// neither count nor verdict, so a profile's count mode could not be checked
// from the command that runs it.
func TestTheTestCommandPrintsTheCountsItStored(t *testing.T) {
	fixture := fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	runner := filepath.Join(t.TempDir(), "runner.sh")
	require.NoError(t, os.WriteFile(runner, []byte(
		"#!/bin/sh\necho 'Tests:  2 passed' >&2\necho 'Tests:  1 failed' >&2\nexit 1\n"), 0o700))

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"tests":{"cmd":["`+runner+`"],"globs":["*_test.txt"],`+
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

	stdout, _ := streams(t, "test", fixturePR, "--repo", fixtureSlug)
	var printed map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &printed))

	assert.Equal(t, float64(3), printed["tests_run"], "the executed count §5.2.4 stored")
	assert.Equal(t, float64(1), printed["tests_failed"], "the failed count §5.2.4 stored")
	assert.Equal(t, false, printed["passed"], "§5.2.5's verdict on a run that failed one test")
}

// §5.2.4's output_tail reaches the command's own document, and `--compact`
// takes it out (§12.5). Measured on tarfin-labs/backend#6328 with cr 0.13.0:
// runs.ndjson held the tail and `cr test`'s JSON carried none, so the output
// the counts were read from could not be seen from the command that ran it.
func TestTheTestCommandPrintsTheOutputTail(t *testing.T) {
	countedTestHome(t, "#!/bin/sh\necho 'line one'\necho 'Tests:  2 passed'\n")
	assert.Equal(t, "line one\nTests:  2 passed\n", printedTestRun(t)["output_tail"])
	assert.NotContains(t, printedTestRun(t, "--compact"), "output_tail")
}

// A count the patterns could not derive is left out of the output and said in
// words in the rendering, never printed as a zero nothing measured.
func TestTheTestRenderingSaysWhenTheCountsWereNotDerived(t *testing.T) {
	three, one := 3, 1
	derived := (&testRunResult{TestsRun: &three, TestsFailed: &one}).Text(&writer{})
	assert.Contains(t, derived, "counts  3 ran, 1 failed")

	underived := (&testRunResult{}).Text(&writer{})
	assert.Contains(t, underived, "counts  not derived")

	encoded, err := json.Marshal(&testRunResult{})
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "tests_run", "absent rather than null when underivable")
}

// §5.2.3 through the command: the budget the run is bounded by is the
// profile's `tests.timeout_seconds`, and a run that exceeds it is killed and
// recorded as a timeout.
//
// The profile sets one second and the runner sleeps for two minutes, so the
// wiring is what is under test rather than the killing: a command that reached
// for §2.4's 900-second default, or for no budget at all, would sit here until
// the suite's own deadline. `timed_out` is asserted in the stored record
// because §5.2.4 has no other slot for the outcome — a process cr killed
// reports the platform's number for a killed process, which is the same shape
// a runner that decided to fail produces.
func TestARunThatExceedsTheProfilesTimeoutIsRecordedAsATimeout(t *testing.T) {
	fixture := fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	runner := filepath.Join(t.TempDir(), "runner.sh")
	require.NoError(t, os.WriteFile(runner, []byte("#!/bin/sh\nsleep 120\n"), 0o700))

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"tests":{"cmd":["`+runner+`"],"globs":["*_test.txt"],"timeout_seconds":1}}`))
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
	require.NoError(t, json.Unmarshal(
		[]byte(afterHeader(t, throughAPipe(t, "test", fixturePR, "--repo", fixtureSlug))), &reported))
	assert.Equal(t, true, reported["timed_out"],
		"§5.2.3: the reader is told the run was killed, not left to read exit -1")

	body, err := os.ReadFile(prepared.PRFile(
		fixtureOwner, fixtureProject, fixturePRNumber, state.FileRuns))
	require.NoError(t, err)
	var stored map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(string(body))), &stored))
	assert.Equal(t, true, stored["timed_out"], "§5.2.4 stores the outcome §5.2.3 mandates")
}

// The terminal rendering says when a run was killed for exceeding its budget.
//
// The exit code printed beside it cannot say so: it is the platform's number
// for a killed process, and a reader who sees only a non-zero exit reads a
// suite that ran and failed. §5.3.4 puts the two on different rungs, and the
// one that never finished supports no grade at all.
func TestTheTestRenderingSaysWhenARunWasKilledForTimingOut(t *testing.T) {
	render := func(t *testing.T, timedOut bool) string {
		t.Helper()
		var printed bytes.Buffer
		out := &writer{out: &printed, mode: ModeText}
		require.NoError(t, out.emit(&testRunResult{
			Run: "r1", Sandbox: "/tmp/sandbox", Command: []string{"pest"},
			ExitCode: -1, TimedOut: timedOut,
			Warnings: []string{}, Honesty: []string{},
		}))
		return printed.String()
	}

	assert.Contains(t, render(t, true), "killed for exceeding tests.timeout_seconds")
	assert.NotContains(t, render(t, false), "killed",
		"a run that finished says nothing about a budget it stayed inside")
}

// Round 12's baseline-contamination finding: §5.1.7 voids a probe whose
// post-run cleanliness check fails, and nothing voided a plain `cr test` run —
// so a suite that dirtied a tracked file under itself could be stored
// `passed: true` and later resolved as the baseline a probe is graded against.
//
// The runner here passes on every one of §5.2.5's three clauses: it exits 0,
// four tests ran, and none failed. What denies it the verdict is the fourth
// thing, and only that, so the record proves the void rather than the counts.
func TestATestRunItsSandboxContaminatedIsNoBaseline(t *testing.T) {
	fixture := fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	// The runner reports a clean pass and then edits a tracked file of the
	// sandbox it is standing in, which is exactly the shape §5.1.6 calls
	// unclean and the shape a suite with a careless fixture really has.
	runner := filepath.Join(t.TempDir(), "runner.sh")
	require.NoError(t, os.WriteFile(runner, []byte(
		"#!/bin/sh\necho 'Tests:  4 passed'\necho 'dirtied' >> app.go\nexit 0\n"), 0o700))

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"tests":{"cmd":["`+runner+`"],"globs":["*_test.txt"],`+
		`"count_pattern":"Tests:  ([0-9]+) passed","failed_pattern":"([0-9]+) failed"}}`))
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, ProfileID: "qa", Round: 2, Head: head,
	}))
	require.NoError(t, held.Unlock())

	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })

	shown := afterHeader(t, throughAPipe(t, "test", fixturePR, "--repo", fixtureSlug))
	var reported map[string]any
	require.NoError(t, json.Unmarshal(
		[]byte(strings.TrimPrefix(shown, "Tests:  4 passed\n")), &reported))
	contaminated, ok := reported["contaminated"].(string)
	require.True(t, ok, "§5.1.6's check names what it found: %v", reported)
	assert.Contains(t, contaminated, "app.go",
		"§5.1.6: the check names the tracked file that differs")

	body, err := os.ReadFile(prepared.PRFile(
		fixtureOwner, fixtureProject, fixturePRNumber, state.FileRuns))
	require.NoError(t, err)
	var stored map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(string(body))), &stored))
	assert.InDelta(t, 0, stored["exit_code"], 0, "the runner itself reported success")
	assert.InDelta(t, 4, stored["tests_run"], 0, "§5.2.5's second clause held")
	assert.InDelta(t, 0, stored["tests_failed"], 0, "§5.2.5's third clause held")
	assert.Equal(t, true, stored["contaminated"], "§5.1.6's check failed after the run")
	assert.Equal(t, false, stored["passed"],
		"a run of a sandbox that drifted from the head is nobody's baseline")
}

// The terminal half of round 12's baseline-contamination finding: a reader
// watching the command has to be told why a run that exited 0 with four tests
// passing was recorded `passed: false`.
//
// Asserted here rather than only in the JSON payload because the two are
// separate renderings, and gremlins said so: negating the condition that prints
// this line survived every test in the package, which means a run could have
// been voided in silence on a terminal while the stored record said otherwise.
func TestTheTestRenderingSaysWhenASandboxContaminatedARun(t *testing.T) {
	render := func(t *testing.T, contaminated string) string {
		t.Helper()
		var printed bytes.Buffer
		out := &writer{out: &printed, mode: ModeText}
		require.NoError(t, out.emit(&testRunResult{
			Run: "r1", Sandbox: "/tmp/sandbox", Command: []string{"pest"},
			Contaminated: contaminated,
			Warnings:     []string{}, Honesty: []string{},
		}))
		return printed.String()
	}

	said := render(t, "tracked files differ from the post-setup baseline: app.go")
	assert.Contains(t, said, "contaminated, per §5.1.6")
	assert.Contains(t, said, "app.go", "the reason names the file, not just the fact")
	assert.NotContains(t, render(t, ""), "contaminated",
		"a run whose sandbox held says nothing about a check it passed")
}
