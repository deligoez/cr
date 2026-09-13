package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/run"
	"github.com/deligoez/cr/internal/state"
)

// The file the fixture's head branch holds, and the mutation of it §5.3.1
// describes: a unified diff that breaks production code the suite is expected
// to notice.
const (
	fixtureSource  = "package app\n\nfunc Retry() { backoff() }\n"
	fixtureMutated = "package app\n\nfunc Retry() {}\n"
	fixtureDiff    = "--- a/app.go\n+++ b/app.go\n@@ -1,3 +1,3 @@\n package app\n \n" +
		"-func Retry() { backoff() }\n+func Retry() {}\n"
)

// probeFixture prepares a pull request `cr probe run` can be pointed at, and
// returns the state layout, the repository under review, the sandbox path, and
// the log the runner writes to.
//
// The runner copies the file under mutation into that log before printing its
// recap, which is the only way to see what the suite actually ran against: a
// mutation that is applied and reverted leaves the sandbox exactly as it found
// it, so a test that looked only at the disk afterwards could not tell a probe
// that mutated nothing from one that mutated and put it back.
//
// tests is spliced into the profile's `tests` object, so a case that needs a
// different budget or a different pattern says so where it is written rather
// than through a second fixture.
//
// The count pattern reads a failed recap as well as a passing one. §5.3.4's
// fifth rung answers an undetermined count with `inconclusive`, so a profile
// that could only count passes would give every failing mutation run that
// answer and the ladder's last two rungs would be unreachable.
func probeFixture(
	t *testing.T, runner string, tests ...string,
) (prepared state.Layout, fixture, sandboxPath, log string) {
	t.Helper()
	fixture = fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	scripts := t.TempDir()
	log = filepath.Join(scripts, "observed.log")
	script := filepath.Join(scripts, "runner.sh")
	require.NoError(t, os.WriteFile(script,
		[]byte("#!/bin/sh\ncat app.go >> "+log+"\necho '---' >> "+log+"\n"+runner), 0o700))

	prepared = state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"tests":{"cmd":["`+script+`"],"globs":["*_test.txt"],"filter_flag":"--only",`+
		`"count_pattern":"Tests:  ([0-9]+) (?:failed|passed)",`+
		`"failed_pattern":"Tests:  ([0-9]+) failed"`+
		strings.Join(append([]string{""}, tests...), ",")+`}}`))
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

	// §9.3.1 has every command that reads per-PR state compare the round's
	// head against the one GitHub reports, so a probe now asks. The shim is
	// put on the PATH rather than only in the in-process seam because three
	// of these tests spawn the built binary — a real SIGKILL is what
	// §5.3.3's second sentence is tested by — and a spawned cr has no seam
	// to install.
	base := strings.TrimSpace(mustGit(t, fixture, "rev-parse", "main"))
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	return prepared, fixture, prepared.Sandbox(fixtureOwner, fixtureProject, fixturePRNumber), log
}

// writePatch stores a unified diff outside the repository under review and
// returns its path.
func writePatch(t *testing.T, diff string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mutation.diff")
	require.NoError(t, os.WriteFile(path, []byte(diff), 0o600))
	return path
}

// storedRecords reads one NDJSON file of the pull request's state back as
// documents, so an assertion is made against what reached disk rather than
// against a struct the test and the writer share.
func storedRecords(t *testing.T, l state.Layout, name string) []map[string]any {
	t.Helper()
	body, err := os.ReadFile(l.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, name))
	require.NoError(t, err)
	records := make([]map[string]any, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

// §5.3.2 end to end: `cr probe run --kind mutation --patch <file>` applies the
// mutation, runs the tests, reverts the mutation, and records the result.
//
// All four steps are asserted, and each of them from something the command
// could not have faked. The apply and the revert are read off the runner's own
// log and the sandbox afterwards — the suite saw the broken function, and the
// file that is on disk when the command returns is the one the head holds. The
// run is the baseline and the mutated run that reached runs.ndjson, in that
// order, which is §5.2.6's "before the probe". And the result is the probe
// record, carrying the value §5.3.4's ladder produced for a suite that noticed
// nothing.
//
// The baseline is what makes that value mean anything. §5.3.5 lets only
// `no-test-failed` prove a gap, and only when the baseline passed — so the two
// records are asserted together, and the mutated run is asserted to carry the
// probe's id, which is the fence §5.2.6 puts around every later baseline.
func TestAMutationProbeAppliesRunsRevertsAndRecords(t *testing.T) {
	prepared, _, sandboxPath, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
	patch := writePatch(t, fixtureDiff)

	// The runner's own output reaches the reader as it is produced, ahead
	// of the document §12.1 keeps stdout for, so the two are separated
	// here rather than the runner being silenced. Two recaps, because
	// §5.2.6 performs the baseline before the probe.
	const recap = "Tests:  4 passed\n"
	shown := throughAPipe(t,
		"probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", patch)
	require.True(t, strings.HasPrefix(shown, recap+recap),
		"§5.2.6: the baseline runs before the probe, and both reach the reader: %q", shown)

	var reported map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(shown, recap+recap)), &reported))

	// The mutation was in place while the tests ran, and only then. The
	// baseline of §5.2.2 goes first and sees the file as the head holds it.
	observed, err := os.ReadFile(log)
	require.NoError(t, err, "the profile's test command never ran")
	assert.Equal(t,
		fixtureSource+"---\n"+fixtureMutated+"---\n", string(observed),
		"§5.2.6's baseline runs on un-probed code, and §5.3.2's probe on the mutation")

	// §5.3.3: the mutation is off disk again when the command returns.
	restored, err := os.ReadFile(filepath.Join(sandboxPath, "app.go"))
	require.NoError(t, err)
	assert.Equal(t, fixtureSource, string(restored),
		"§5.3.3: the sandbox holds what the head holds once the probe is done")

	assert.Equal(t, "p1", reported["probe"])
	assert.Equal(t, "mutation", reported["kind"])
	assert.Equal(t, "no-test-failed", reported["result"])
	assert.Equal(t, "gap", reported["establishes"],
		"§5.3.5: the one result that proves a gap, against a baseline that passed")
	assert.Equal(t, "r1", reported["baseline"])
	assert.Equal(t, "r2", reported["run"])

	runs := storedRecords(t, prepared, state.FileRuns)
	require.Len(t, runs, 2, "§5.2.6 performs the baseline before the probe, and both are recorded")
	assert.Equal(t, "r1", runs[0]["id"])
	assert.NotContains(t, runs[0], "probe",
		"§5.2.6: only a run carrying no probe may serve as a baseline")
	assert.Equal(t, true, runs[0]["passed"], "§5.2.5's verdict on un-probed code")
	assert.Equal(t, "r2", runs[1]["id"])
	assert.Equal(t, "p1", runs[1]["probe"], "the mutated run names the probe it measured")

	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 1, "§5.5: one probe record per probe run")
	stored := probes[0]
	duration, ok := stored["duration_ms"].(float64)
	require.True(t, ok, "§5.5 stores the wall-clock duration")
	assert.GreaterOrEqual(t, duration, float64(0))
	delete(stored, "duration_ms")
	assert.Equal(t, map[string]any{
		"id":           "p1",
		"head":         runs[0]["head"],
		"round":        float64(2),
		"kind":         "mutation",
		"input":        fixtureDiff,
		"result":       "no-test-failed",
		"target":       "app.go:3",
		"tests_run":    float64(4),
		"tests_failed": float64(0),
		"baseline":     "r1",
		"output_tail":  "Tests:  4 passed\n",
	}, stored, "§5.5: the record says which experiment ran and what it was measured against")
}

// §5.3.2: cr rejects `--target` for a mutation probe, and nothing runs.
//
// The refusal is what keeps §5.3.2's last sentence true — "the evidence chain
// from experiment to assertion then runs on the patch cr executed, not on a
// flag the agent typed". A supplied target would be the one field of the record
// that came from the agent's judgement rather than from the experiment, and
// §6.2.2 has a `probed` finding's evidence point at exactly that field.
//
// Nothing having run is asserted as well as the exit code. Accepting the flag
// and ignoring it would produce the same output document, and would be the
// worse failure: the agent would have named a line and been told nothing.
func TestAMutationProbeRefusesASuppliedTarget(t *testing.T) {
	_, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
	patch := writePatch(t, fixtureDiff)

	err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", patch, "--target", "app.go:3")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--target is rejected for a mutation probe")
	assert.Contains(t, err.Error(), "§5.3.2")
	assert.Equal(t, ExitUsage, exitCodeFor(err),
		"§11.2 codes a flag this kind does not take as a malformed invocation")
	assert.NoFileExists(t, log, "the refusal comes before any suite is run")
}

// Round 12's unbounded-patch-target finding: a patch whose paths resolve
// outside the sandbox is refused, so §5.1.4's invariant is enforced rather than
// assumed.
//
// The sandbox is a worktree of the repository under review, so one `..` in an
// agent-written diff is all it takes to arrive in the checkout the reviewer is
// working in — which invariant 2 permits cr no write into at all. The refusal
// lands before anything is run, which the absent runner log is what proves:
// §5.2.6 would otherwise perform a whole baseline suite for a patch cr was
// never going to apply.
func TestAPatchAimedOutOfTheSandboxIsRefused(t *testing.T) {
	_, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
	patch := writePatch(t,
		"--- a/../../escaped.go\n+++ b/../../escaped.go\n@@ -1 +1 @@\n-one\n+two\n")

	err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", patch)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "§5.1.4")
	assert.Contains(t, err.Error(), "it leaves the tree it applies to")
	assert.Equal(t, ExitValidation, exitCodeFor(err),
		"§11.2 codes the agent's data inside a file it could read as a validation failure")
	assert.NoFileExists(t, log, "the refusal comes before any suite is run")
}

// onlyWhenMutated is a runner that behaves one way against the head's own file
// and another against the mutated one.
//
// §5.2.6 performs a baseline before the probe, through the same command, so a
// runner that hung or failed unconditionally would do it twice — once on
// un-probed code, where a failing baseline would deny the probe §5.3.5's
// footing before the case under test was reached.
func onlyWhenMutated(then string) string {
	return "if grep -q 'func Retry() {}' app.go; then\n" + then + "fi\necho 'Tests:  4 passed'\n"
}

// spawnedProbe is a `cr probe run` started as a process of its own, for the
// tests that kill it: what it printed and how it ended are kept, so a wait for
// it to reach a state on disk can say why it never got there.
type spawnedProbe struct {
	cmd     *exec.Cmd
	output  bytes.Buffer
	started time.Time
	done    chan struct{}
	err     error
}

// runAsCR names the variable that makes this package's test binary run as cr:
// TestMain sees it and calls Execute, which is all cmd/cr's main does.
const runAsCR = "CRTEST_RUN_AS_CR"

// spawnProbe starts `cr probe run` on the fixture's pull request with args, as
// a process of its own, on an environment holding only what cr needs.
//
// The process is this test binary re-executed as cr rather than a binary built
// from the working tree when the test reaches this line. Every recorded failure
// of the two killed-probe tests came from a tree holding uncommitted edits, not
// from load. The short ones failed within a second, and the one that printed
// its cause was `go build ./cmd/cr` failing on a half-written file; the long
// ones waited out the whole minute because the spawned cr had already exited
// (it had started asking gh, and its PATH held no shim) and the old wait could
// only time out, printing nothing the process said. Thirty runs
// under a parallel `go test ./...` reached the state on disk in under a second
// every time. Re-executing runs exactly the code the test was compiled with,
// through the same Execute, and a SIGKILL ends it the same way; awaitOnDisk
// reports a process that died instead of waiting for it.
func spawnProbe(t *testing.T, prepared state.Layout, fixture string, args ...string) *spawnedProbe {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)
	p := &spawnedProbe{done: make(chan struct{})}
	p.cmd = exec.Command(self,
		append([]string{"probe", "run", fixturePR, "--repo", fixtureSlug}, args...)...)
	p.cmd.Dir = fixture
	p.cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"TMPDIR=" + os.TempDir(),
		state.HomeEnv + "=" + prepared.Root(),
		runAsCR + "=1",
	}
	p.cmd.Stdout, p.cmd.Stderr = &p.output, &p.output
	require.NoError(t, p.cmd.Start())
	p.started = time.Now()
	go func() {
		p.err = p.cmd.Wait()
		close(p.done)
	}()
	t.Cleanup(func() { p.stop() })
	return p
}

// stop kills the process and waits for Wait to return, after which output and
// err are the process's last word and safe to read.
func (p *spawnedProbe) stop() {
	_ = p.cmd.Process.Kill()
	<-p.done
}

// kill is the SIGKILL the killed-probe tests exist to deliver.
func (p *spawnedProbe) kill(t *testing.T) {
	t.Helper()
	select {
	case <-p.done:
		require.FailNowf(t, "cr probe run ended before it was killed",
			"it exited %v after %s and printed:\n%s", p.err, time.Since(p.started), p.output.String())
	default:
	}
	p.stop()
}

// awaitOnDisk waits up to a minute for reached to hold. A process that exits
// first fails the test at once with its exit status and everything it printed,
// and one still running at the deadline is killed and reported the same way:
// a wait that only timed out could not tell a slow probe from a dead one.
func (p *spawnedProbe) awaitOnDisk(t *testing.T, what string, reached func() bool) {
	t.Helper()
	deadline := time.After(60 * time.Second)
	for !reached() {
		select {
		case <-p.done:
			if reached() {
				return
			}
			require.FailNowf(t, what, "cr probe run exited %v after %s and printed:\n%s",
				p.err, time.Since(p.started), p.output.String())
		case <-deadline:
			p.stop()
			require.FailNowf(t, what, "cr probe run was still running after %s and printed:\n%s",
				time.Since(p.started), p.output.String())
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Logf("cr probe run reached the state on disk %s after it started", time.Since(p.started))
}

// §5.3.3 and invariant 6: the mutation is reverted when the run fails.
//
// A failing mutation run is §5.3.4's last rung and §5.3.7's disproof, so it is
// the outcome an agent is most likely to reach on the way to deciding there is
// no gap — and the one where a revert written after the run would be skipped by
// the error return above it. The sandbox is asserted to be back at the head's
// own file, and the record to carry the ladder's answer rather than §5.1.7's
// override, which is what says §5.1.6's post-run check passed on a sandbox the
// probe had put back.
func TestTheMutationIsRevertedAfterAFailingRun(t *testing.T) {
	prepared, _, sandboxPath, _ := probeFixture(t,
		onlyWhenMutated("  echo 'Tests:  1 failed'\n  exit 1\n"))
	patch := writePatch(t, fixtureDiff)

	shown := runProbe(t, patch)
	assert.Equal(t, "failed", shown["result"],
		"§5.3.4's last rung: the suite noticed the mutation")
	assert.Equal(t, "no-gap", shown["establishes"],
		"§5.3.7: a failed result disproves the gap, and the agent does not raise the finding")
	assert.Empty(t, shown["voided"], "§5.1.6's post-run check passed, so §5.1.7 overrode nothing")

	restored, err := os.ReadFile(filepath.Join(sandboxPath, "app.go"))
	require.NoError(t, err)
	assert.Equal(t, fixtureSource, string(restored),
		"§5.3.3: the mutation is reverted even when the run fails")

	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 1)
	assert.Equal(t, "failed", probes[0]["result"])
}

// §5.3.3 and invariant 6: the mutation is reverted when the run times out.
//
// This is the case that most obviously escapes a revert placed after the run,
// because the run does not end — cr kills it. §5.2.3's budget is one second and
// the runner sleeps for two minutes against the mutated file, so what is
// measured is the kill and the revert that follows it, not a suite that
// happened to finish first.
func TestTheMutationIsRevertedAfterATimeout(t *testing.T) {
	prepared, _, sandboxPath, _ := probeFixture(t,
		onlyWhenMutated("  sleep 120\n"), `"timeout_seconds":1`)
	patch := writePatch(t, fixtureDiff)

	shown := runProbe(t, patch)
	assert.Equal(t, "timeout", shown["result"],
		"§5.3.4's second rung: a run that never finished said nothing about the code")
	assert.Equal(t, "nothing", shown["establishes"],
		"§5.3.5: a run that never finished cannot manufacture evidence")

	restored, err := os.ReadFile(filepath.Join(sandboxPath, "app.go"))
	require.NoError(t, err)
	assert.Equal(t, fixtureSource, string(restored),
		"§5.3.3: the mutation is reverted even when the run times out")

	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 1)
	assert.Equal(t, "timeout", probes[0]["result"])
}

// §5.3.3's third case, and the one no code inside cr can answer: cr is killed
// while the mutation is on disk, and §5.1.6's check on the next invocation
// detects the unclean sandbox and recreates it before running anything.
//
// A real SIGKILL to a real `cr probe run`, because the failure being tested is
// the process ceasing to exist: a deferred revert, a signal handler, and an
// atexit hook are all equally absent afterwards, so nothing short of killing
// the binary tests what §5.3.3's second sentence is for. The mutation is
// asserted to be on disk after the kill — otherwise the recreation below would
// be recreating a sandbox that was already clean, and the test would pass
// meaning nothing.
func TestAKilledProbeLeavesASandboxTheNextRunRecreates(t *testing.T) {
	prepared, fixture, sandboxPath, _ := probeFixture(t, onlyWhenMutated("  sleep 30\n"))
	patch := writePatch(t, fixtureDiff)
	mutated := filepath.Join(sandboxPath, "app.go")

	probing := spawnProbe(t, prepared, fixture,
		"--kind", "mutation", "--patch", patch)
	probing.awaitOnDisk(t, "the probe never applied its mutation", func() bool {
		held, err := os.ReadFile(mutated)
		return err == nil && string(held) == fixtureMutated
	})
	probing.kill(t)

	held, err := os.ReadFile(mutated)
	require.NoError(t, err)
	require.Equal(t, fixtureMutated, string(held),
		"the killed run left the mutation behind, which is what the next run has to find")

	// The next invocation, which is what §5.3.3's second sentence hands the
	// job to. `cr test` is used rather than a second probe because the
	// check is §5.1.6's and belongs to every run: whichever command comes
	// next has to rebuild the sandbox before it measures anything.
	var reported map[string]any
	shown := throughAPipe(t, "test", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, json.Unmarshal(
		[]byte(strings.TrimPrefix(shown, "Tests:  4 passed\n")), &reported))

	honesty, ok := reported["honesty"].([]any)
	require.True(t, ok, "§11.1's disclosures are a field on the payload")
	require.Len(t, honesty, 1, "§5.1.6: the sandbox was unclean, so it was recreated and said so")
	assert.Contains(t, honesty[0], "§5.1.6")
	assert.Contains(t, honesty[0], "app.go",
		"the notice names the tracked file the killed probe left changed")

	rebuilt, err := os.ReadFile(mutated)
	require.NoError(t, err)
	assert.Equal(t, fixtureSource, string(rebuilt),
		"§5.1.6: the recreated sandbox holds what the head holds")
}

// A `--patch` that holds no hunk is refused, and the refusal names the flag
// that produces one.
//
// This is not a hypothetical malformed file. A machine whose owner has set
// `diff.external` — one of this project's own does — answers `git diff` with
// that tool's output, which is not a unified diff at all, and an agent handing
// cr the result has done nothing wrong. cr's own reads pin `--no-ext-diff` and
// the agent writing the patch is outside that fence, so the refusal is where
// the flag has to be named. §12.4 asks every error for the next actionable
// step, and "this is not a diff" is not one.
func TestAPatchHoldingNoHunkNamesTheFlagThatWritesOne(t *testing.T) {
	_, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
	// What `git diff` writes when diff.external is configured: a report
	// about the file, with no hunk header anywhere in it.
	patch := writePatch(t, "src/Order.php --- PHP\n34   return 0.0;   34   return 999.0;\n")

	err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", patch)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "holds no hunk")
	assert.Contains(t, err.Error(), "--no-ext-diff")
	assert.Contains(t, err.Error(), "diff.external")
	assert.NoFileExists(t, log, "the refusal comes before any suite is run")
}

// §5.3.4's first rung through the command: a patch that does not apply cleanly
// records `error`, runs no tests, and leaves the sandbox as it found it.
//
// It is a probe result rather than a command failure, because the agent asked
// what the suite says about this mutation and cr can answer that the mutation
// was never made. The sandbox is asserted because the apply is where a patch
// can fail halfway — the first hunk lands, the second does not — and a rung 1
// recorded over a half-mutated checkout would be §5.1.6's problem on the next
// run rather than this one's.
func TestAPatchThatDoesNotApplyIsRecordedWithoutRunningAnything(t *testing.T) {
	prepared, _, sandboxPath, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
	// The context line is the head's own, and the removed line is not, so
	// the hunk's arithmetic is right and its content is stale.
	patch := writePatch(t, "--- a/app.go\n+++ b/app.go\n@@ -1,3 +1,3 @@\n package app\n \n"+
		"-func Retry() { rewritten() }\n+func Retry() {}\n")

	shown := runProbe(t, patch)
	assert.Equal(t, "error", shown["result"],
		"§5.3.4's first rung: the mutation was never made")
	assert.Empty(t, shown["run"], "§5.3.4's first rung runs no tests, so there is no run to record")

	restored, err := os.ReadFile(filepath.Join(sandboxPath, "app.go"))
	require.NoError(t, err)
	assert.Equal(t, fixtureSource, string(restored))

	// §5.2.6's baseline still ran, which is why the log is here: the
	// baseline comes before the probe and is not the probe.
	observed, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, fixtureSource+"---\n", string(observed),
		"the suite ran once, for the baseline, and not for a mutation that was never applied")

	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 1)
	assert.Equal(t, "error", probes[0]["result"])
	assert.Equal(t, "app.go:3", probes[0]["target"],
		"§5.5's target is derived from the patch even when the patch would not apply")
	runs := storedRecords(t, prepared, state.FileRuns)
	require.Len(t, runs, 1, "only §5.2.6's baseline reached runs.ndjson")
	assert.NotContains(t, runs[0], "probe")
}

// §5.1.7 through the command: a probe whose post-run cleanliness check fails is
// recorded `error` whatever the ladder produced, grades nothing, and forces the
// sandbox to be recreated before the next run.
//
// The runner passes cleanly and dirties a tracked file the mutation does not
// touch, so the revert cannot put it back — which is the shape a suite with a
// careless fixture really has. The ladder would have answered `no-test-failed`,
// the one mutation result §5.3.5 lets prove a gap, so the override is the
// difference between a record that licenses an assertion to a colleague and one
// that licenses nothing.
func TestAProbeWhoseSandboxFailedItsCheckIsVoidedAndForcesRecreation(t *testing.T) {
	// Only the mutated run dirties the file. §5.2.6's baseline runs first
	// through the same runner, and a baseline that contaminated itself would
	// stand as nobody's baseline — so the probe would be refused for want of
	// a foundation rather than voided by §5.1.7, which is a different
	// sentence about a different failure.
	prepared, _, sandboxPath, _ := probeFixture(t,
		onlyWhenMutated("  echo 'dirtied by the suite' >> .gitignore\n"))
	patch := writePatch(t, fixtureDiff)

	shown := runProbe(t, patch)
	assert.Equal(t, "error", shown["result"],
		"§5.1.7: the check overrides what §5.3.4's ladder produced")
	assert.Equal(t, "nothing", shown["establishes"],
		"§5.1.7: such a probe establishes nothing in either direction")
	voided, ok := shown["voided"].(string)
	require.True(t, ok, "§5.1.7's reason reaches the reader: %v", shown)
	assert.Contains(t, voided, ".gitignore", "the reason names the file that differs")

	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 1, "§5.5.1: one record is written, and it is never amended")
	assert.Equal(t, "error", probes[0]["result"])
	runs := storedRecords(t, prepared, state.FileRuns)
	require.Len(t, runs, 2)
	assert.Equal(t, true, runs[1]["contaminated"],
		"the mutated run measured a sandbox that had drifted under it")

	// §5.1.7's third consequence, read off the next invocation rather than
	// off a flag: the sandbox is rebuilt before anything else runs.
	var reported map[string]any
	next := throughAPipe(t, "test", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, json.Unmarshal(
		[]byte(strings.TrimPrefix(next, "Tests:  4 passed\n")), &reported))
	honesty, ok := reported["honesty"].([]any)
	require.True(t, ok)
	require.Len(t, honesty, 1, "§5.1.7: recreation is forced before the next run")
	assert.Contains(t, honesty[0], "§5.1.6")

	dropped, err := os.ReadFile(filepath.Join(sandboxPath, ".gitignore"))
	require.NoError(t, err)
	assert.NotContains(t, string(dropped), "dirtied by the suite",
		"the rebuilt sandbox is a fresh checkout")
}

// The terminal rendering says when §5.1.7 voided a probe, and says nothing
// about it when the check passed.
//
// Asserted apart from the JSON payload because the two are separate renderings
// and a reader watching the command has to be told why a result of `error`
// stands where the ladder's own answer would have gone.
func TestTheProbeRenderingSaysWhenTheCheckVoidedTheResult(t *testing.T) {
	render := func(t *testing.T, voided string) string {
		t.Helper()
		var printed bytes.Buffer
		out := &writer{out: &printed, mode: ModeText}
		require.NoError(t, out.emit(&probeRunResult{
			Probe: "p1", Kind: "mutation", Sandbox: "/tmp/sandbox",
			Command: []string{"pest"}, Result: "error", Target: "app.go:3",
			Baseline: "r1", Voided: voided,
			Warnings: []string{}, Honesty: []string{},
		}))
		return printed.String()
	}

	said := render(t, "a probe artefact was left behind: tests/cr_probe_p1Test.php")
	assert.Contains(t, said, "voided, per §5.1.7")
	assert.Contains(t, said, "cr_probe_p1Test.php", "the reason names what was found")
	assert.NotContains(t, render(t, ""), "voided",
		"a probe whose sandbox held says nothing about a check it passed")
}

// runProbe runs one mutation probe through the command and returns the document
// it printed, with the runner's own output separated off.
//
// The runner writes a recap for §5.2.6's baseline and, unless it was killed, one
// for the probe as well; §12.1 keeps stdout for the document, and the test
// harness gives the command one file for both streams.
func runProbe(t *testing.T, patch string) map[string]any {
	t.Helper()
	return probeDocument(t, throughAPipe(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", patch))
}

// probeDocument separates the command's own document from the runner output
// printed ahead of it, and returns the document.
func probeDocument(t *testing.T, shown string) map[string]any {
	t.Helper()
	document := strings.Index(shown, "{\n")
	require.GreaterOrEqual(t, document, 0, "the command printed no document: %q", shown)

	var reported map[string]any
	require.NoError(t, json.Unmarshal([]byte(shown[document:]), &reported))
	return reported
}

// The gap probe fixture: where §2.4's template puts the file, the test file
// §5.4.1 has the agent supply, and a runner that prints whether it could see
// one.
//
// The template names a directory the sandbox does not hold, which is the case
// worth fixing in the fixture rather than the easy one: §2.4 roots the default
// at the first `tests.globs` entry, so a profile whose tests live in a
// directory is the ordinary shape, and the placement has to make that directory
// and the removal has to take it away again.
const (
	gapProbeTemplate = `"probe_path_template":"tests/cr_probe_<probe-id>.txt"`
	gapProbePath     = "tests/cr_probe_p1.txt"
	gapProbeTest     = "the supplied test, asserting the edge case\n"
	gapProbeRunner   = "cat " + gapProbePath + " 2>/dev/null || echo 'the probe file is not there'\n" +
		"echo 'Tests:  5 passed'\n"
)

// writeProbeTest stores §5.4.1's test file outside the repository under review
// and returns its path.
func writeProbeTest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "edge_case_test.txt")
	require.NoError(t, os.WriteFile(path, []byte(gapProbeTest), 0o600))
	return path
}

// §5.4.4 and §5.4.5 as `cr probe run` reports them over §5.4.3's whole
// vocabulary: what the reader is told the run established.
//
// Only `failed` is left undecided, and that is the narrowing this test exists
// for. Reporting `undecided` for all six would be honest about `failed` and
// misleading about the other five: §5.4.5 settles them at the run itself, and a
// reader told the question is open goes looking for a claim to map when there
// is nothing a mapping could change.
//
// `passed` is its own answer rather than `nothing`, because §5.4.5 says the run
// established the behaviour is present — and then says in the same breath that
// this is not the missing test §5.3's `no-test-failed` establishes.
//
// Every token is asserted to carry a clause as well. The clause is what a
// terminal reader actually sees, so a token added without one would print an
// empty line where the evidence should be.
func TestWhatEachGapResultIsReportedToEstablish(t *testing.T) {
	for _, tc := range []struct {
		result      probe.Result
		establishes string
	}{
		{result: "failed", establishes: establishesUndecided},
		{result: "passed", establishes: establishesBehaviour},
		{result: "timeout", establishes: establishesNothing},
		{result: "error", establishes: establishesNothing},
		{result: "no-tests-selected", establishes: establishesNothing},
		{result: "inconclusive", establishes: establishesNothing},
	} {
		t.Run(string(tc.result), func(t *testing.T) {
			token := gapEstablishedBy(&probe.Record{Kind: probe.Gap, Result: tc.result})

			assert.Equal(t, tc.establishes, token)
			assert.Contains(t, establishedClause[token], "§",
				"the reader is told which section decided, not just a word")
		})
	}
}

// §5.4.2 end to end: `cr probe run --kind gap --test <file> --target <path:line>`
// places the test at the path `tests.probe_path_template` gives, runs it,
// removes it, and records the result.
//
// All four steps are asserted, and each from something the command could not
// have faked. The placement is read off the runner's own output — the suite
// printed the supplied test's content, which it could only do by finding the
// file — and the baseline that ran first printed that no such file was there,
// which is what says the placement covered the probe run alone. The removal is
// the sandbox afterwards, directory included. And the result is the record,
// carrying the value §5.4.3's ladder produced for a suite that ran the supplied
// test and reported no failure.
//
// `establishes` is `behaviour`, which is §5.4.5's own reading of this run: the
// supplied test passed, so the behaviour it asserts is present. It is not
// `gap` — §5.4.5 says only §5.3's `no-test-failed` establishes a missing test —
// and it is not `undecided` either, because §5.4.4's conditional support is
// read off a `failed` result and this run did not produce one.
func TestAGapProbePlacesRunsRemovesAndRecords(t *testing.T) {
	prepared, _, sandboxPath, _ := probeFixture(t, gapProbeRunner, gapProbeTemplate)
	supplied := writeProbeTest(t)

	reported := probeDocument(t, throughAPipe(t, "probe", "run", fixturePR,
		"--repo", fixtureSlug, "--kind", "gap", "--test", supplied, "--target", "app.go:3"))

	assert.Equal(t, "p1", reported["probe"])
	assert.Equal(t, "gap", reported["kind"])
	assert.Equal(t, "passed", reported["result"],
		"§5.4.3's fifth rung: the supplied test ran and nothing failed")
	assert.Equal(t, "behaviour", reported["establishes"],
		"§5.4.5: the behaviour is present, and that is not a missing test")
	assert.Equal(t, "app.go:3", reported["target"], "§5.5 takes a gap probe's target from --target")
	assert.Equal(t, "r1", reported["baseline"])
	assert.Equal(t, "r2", reported["run"])
	assert.Empty(t, reported["voided"], "§5.1.6's post-run check passed, so §5.1.7 overrode nothing")

	// §5.4.2: the file is off disk again when the command returns, and so
	// is the directory the placement had to make for it. §5.1.6 would find
	// either on the next run and condemn the sandbox for it.
	assert.NoFileExists(t, filepath.Join(sandboxPath, filepath.FromSlash(gapProbePath)),
		"§5.4.2: the test is removed after the run")
	assert.NoDirExists(t, filepath.Join(sandboxPath, "tests"),
		"§5.4.2: the directory the placement created goes with the file")

	runs := storedRecords(t, prepared, state.FileRuns)
	require.Len(t, runs, 2, "§5.2.6 performs the baseline before the probe, and both are recorded")
	assert.Equal(t, "r1", runs[0]["id"])
	assert.NotContains(t, runs[0], "probe",
		"§5.2.6: only a run carrying no probe may serve as a baseline")
	assert.Equal(t, true, runs[0]["passed"], "§5.2.5's verdict on un-probed code")
	assert.Contains(t, runs[0]["output_tail"], "the probe file is not there",
		"§5.5: a gap probe's baseline is the unfiltered run, on which its test does not exist")
	assert.Equal(t, "p1", runs[1]["probe"], "the probe's own run names the probe it measured")

	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 1, "§5.5: one probe record per probe run")
	stored := probes[0]
	duration, ok := stored["duration_ms"].(float64)
	require.True(t, ok, "§5.5 stores the wall-clock duration")
	assert.GreaterOrEqual(t, duration, float64(0))
	delete(stored, "duration_ms")
	assert.Equal(t, map[string]any{
		"id":           "p1",
		"head":         runs[0]["head"],
		"round":        float64(2),
		"kind":         "gap",
		"input":        gapProbeTest,
		"result":       "passed",
		"target":       "app.go:3",
		"tests_run":    float64(5),
		"tests_failed": float64(0),
		"baseline":     "r1",
		"output_tail":  gapProbeTest + "Tests:  5 passed\n",
	}, stored, "§5.5: the record says which experiment ran and what it was measured against")
}

// §5.4.2: an existing file at the probe path aborts with exit code 4, and the
// file that was there is left exactly as it was.
//
// The refusal is not tidiness. §5.4.2 places the test and removes it again, so
// writing over an occupied path would destroy the file and then delete it — and
// the path is required to sit where the profile's `tests.globs` point, which is
// where real tests live.
//
// §5.1.6 cannot catch this, which is what makes the check worth having. Its
// leftover scan is scoped to untracked files (round 12's
// artefact-glob-not-scoped-to-untracked), so a tracked test file at that path
// passes the cleanliness check untouched — and so does one `sandbox.setup`
// creates, which is what this fixture uses because a setup command runs again
// on every recreation and therefore survives one.
func TestAGapProbeRefusesAnExistingFileAtTheProbePath(t *testing.T) {
	prepared, _, sandboxPath, log := probeFixture(t, gapProbeRunner, gapProbeTemplate)
	occupyProbePath(t, prepared)
	supplied := writeProbeTest(t)

	err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "gap", "--test", supplied, "--target", "app.go:3")

	require.Error(t, err)
	assert.Contains(t, err.Error(), gapProbePath+" already exists")
	assert.Contains(t, err.Error(), "§5.4.2")
	assert.Equal(t, ExitState, exitCodeFor(err),
		"§11.2 codes an occupied path the pull request already holds as a state conflict")
	assert.NoFileExists(t, log, "the refusal comes before any suite is run")

	held, err := os.ReadFile(filepath.Join(sandboxPath, filepath.FromSlash(gapProbePath)))
	require.NoError(t, err, "§5.4.2 aborts rather than removing the file it refused to write over")
	assert.Equal(t, fixtureSource, string(held), "the file that was there is untouched")
}

// occupyProbePath gives the fixture's profile a `sandbox.setup` that puts a
// file exactly where §2.4's template resolves.
//
// The profile is edited rather than rewritten, because the runner it names sits
// in a temporary directory only probeFixture knows.
func occupyProbePath(t *testing.T, prepared state.Layout) {
	t.Helper()
	const axes = `"axes":{"test":true},`
	body, err := os.ReadFile(prepared.Profile("qa"))
	require.NoError(t, err)
	require.Contains(t, string(body), axes, "the fixture profile is not the shape this edit expects")
	withSetup := strings.Replace(string(body), axes,
		axes+`"sandbox":{"setup":["mkdir -p tests","cp app.go `+gapProbePath+`"]},`, 1)
	require.NoError(t, os.WriteFile(prepared.Profile("qa"), []byte(withSetup), 0o600))
}

// onlyWithTheProbeFile is a runner that behaves one way while §5.4.2's test
// file is in the sandbox and another way without it.
//
// §5.2.6 performs a baseline before the probe, through the same command and
// with the file not yet placed, so a runner that hung or failed unconditionally
// would do it twice — once on un-probed code, where a failing baseline would
// deny the probe its footing before the case under test was reached.
func onlyWithTheProbeFile(then string) string {
	return "if [ -f " + gapProbePath + " ]; then\n" + then + "fi\necho 'Tests:  5 passed'\n"
}

// runGap runs one gap probe through the command and returns the document it
// printed.
func runGap(t *testing.T, supplied string) map[string]any {
	t.Helper()
	return probeDocument(t, throughAPipe(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "gap", "--test", supplied, "--target", "app.go:3"))
}

// §5.4.2 and invariant 6: the test file is removed when the run fails.
//
// A failing gap probe is §5.4.3's last rung and the outcome §5.4.4 is written
// about, so it is the one an agent most often arrives at — and the one where a
// removal written after the run would be skipped by the error return above it.
// The record is asserted to carry the ladder's answer rather than §5.1.7's
// override, which is what says §5.1.6's post-run check passed on a sandbox the
// probe had cleared.
func TestTheGapProbeFileIsRemovedAfterAFailingRun(t *testing.T) {
	prepared, _, sandboxPath, _ := probeFixture(t,
		onlyWithTheProbeFile("  echo 'Tests:  1 failed'\n  exit 1\n"), gapProbeTemplate)
	supplied := writeProbeTest(t)

	shown := runGap(t, supplied)
	assert.Equal(t, "failed", shown["result"],
		"§5.4.3's last rung: the supplied test ran and failed")
	assert.Empty(t, shown["voided"], "§5.1.6's post-run check passed, so §5.1.7 overrode nothing")

	assert.NoFileExists(t, filepath.Join(sandboxPath, filepath.FromSlash(gapProbePath)),
		"§5.4.2: the removal happens even when the run fails")
	assert.NoDirExists(t, filepath.Join(sandboxPath, "tests"))

	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 1)
	assert.Equal(t, "failed", probes[0]["result"])
}

// §5.4.2 and invariant 6: the test file is removed when the run times out.
//
// This is the case that most obviously escapes a removal placed after the run,
// because the run does not end — cr kills it. §5.2.3's budget is one second and
// the runner sleeps for two minutes once the probe file is there, so what is
// measured is the kill and the removal that follows it, not a suite that
// happened to finish first.
func TestTheGapProbeFileIsRemovedAfterATimeout(t *testing.T) {
	prepared, _, sandboxPath, _ := probeFixture(t,
		onlyWithTheProbeFile("  sleep 120\n"), gapProbeTemplate, `"timeout_seconds":1`)
	supplied := writeProbeTest(t)

	shown := runGap(t, supplied)
	assert.Equal(t, "timeout", shown["result"],
		"§5.4.3's first rung: a run that never finished said nothing about the code")

	assert.NoFileExists(t, filepath.Join(sandboxPath, filepath.FromSlash(gapProbePath)),
		"§5.4.2: the removal happens even when the run times out")
	assert.NoDirExists(t, filepath.Join(sandboxPath, "tests"))

	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 1)
	assert.Equal(t, "timeout", probes[0]["result"])
}

// Round 9's probe-ladder-asymmetry through the command: a runner that exited on
// a signal is §5.4.3's `error` rung, not its `failed` one.
//
// The runner prints a recap `tests.count_pattern` and `tests.failed_pattern`
// both match and then segmentation-faults, which is the shape an out-of-memory
// kill and a crashing extension really have. Read by the counts alone that run
// is the ladder's last rung — and §5.4.4 makes a supported failed gap probe
// carry severity at least `high`, so a crashed process would arrive as the
// loudest item in the draft. The counts are asserted on the record beside the
// result, because they are what the rung overrode.
func TestAGapProbeWhoseRunnerExitedOnASignalIsAnError(t *testing.T) {
	prepared, _, sandboxPath, _ := probeFixture(t,
		onlyWithTheProbeFile("  echo 'Tests:  5 failed'\n  kill -SEGV $$\n"), gapProbeTemplate)
	supplied := writeProbeTest(t)

	shown := runGap(t, supplied)
	assert.Equal(t, "error", shown["result"],
		"§5.4.3's second rung: a runner that died on a signal said nothing about the code")
	assert.Empty(t, shown["voided"],
		"§5.1.6's check passed, so this is the ladder's answer and not §5.1.7's override")

	assert.NoFileExists(t, filepath.Join(sandboxPath, filepath.FromSlash(gapProbePath)),
		"§5.4.2: the removal happens even when the runner comes apart")

	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 1)
	assert.Equal(t, "error", probes[0]["result"])
	assert.Equal(t, float64(5), probes[0]["tests_failed"],
		"the counts the crashed runner printed are recorded, and are what the rung overrode")
}

// §5.4.2's last clause: the removal did not happen, and §5.1.6 catches it.
//
// cr is killed while the probe file is on disk, which is the failure no code
// inside the process can answer — a deferred removal, a signal handler and an
// atexit hook are all equally absent afterwards, so nothing short of killing
// the binary tests what the clause is for. The file is asserted to be there
// after the kill, because otherwise the recreation below would be recreating a
// sandbox that was already clean and the test would pass meaning nothing.
//
// The leftover is untracked and matches `tests.probe_path_template` with its
// `<probe-id>` position replaced by `*`, which is exactly what §5.1.6 scans
// for. The next invocation is `cr test` rather than a second probe, because the
// check belongs to every run: whichever command comes next has to rebuild the
// sandbox before it measures anything.
func TestAKilledGapProbeLeavesAFileTheNextRunRecreates(t *testing.T) {
	prepared, fixture, sandboxPath, _ := probeFixture(t,
		onlyWithTheProbeFile("  sleep 30\n"), gapProbeTemplate)
	supplied := writeProbeTest(t)
	placed := filepath.Join(sandboxPath, filepath.FromSlash(gapProbePath))

	probing := spawnProbe(t, prepared, fixture,
		"--kind", "gap", "--test", supplied, "--target", "app.go:3")
	probing.awaitOnDisk(t, "the probe never placed its test file", func() bool {
		held, err := os.ReadFile(placed)
		return err == nil && string(held) == gapProbeTest
	})
	probing.kill(t)

	require.FileExists(t, placed,
		"the killed run left the probe file behind, which is what the next run has to find")

	reported := probeDocument(t, throughAPipe(t, "test", fixturePR, "--repo", fixtureSlug))

	honesty, ok := reported["honesty"].([]any)
	require.True(t, ok, "§11.1's disclosures are a field on the payload")
	require.Len(t, honesty, 1, "§5.1.6: the sandbox was unclean, so it was recreated and said so")
	assert.Contains(t, honesty[0], "§5.1.6")
	assert.Contains(t, honesty[0], "a probe artefact was left behind")
	assert.Contains(t, honesty[0], "cr_probe_p1.txt",
		"the notice names the file the killed probe left behind")

	assert.NoFileExists(t, placed, "§5.1.6: the recreated sandbox holds what the head holds")
	assert.NoDirExists(t, filepath.Join(sandboxPath, "tests"))
}

// §5.4.2's reservation and §5.5.2's id space: an id another run spent while
// this probe was running is refused rather than shared.
//
// A gap probe has to know its id before it runs, because §2.4's template puts
// that id in the path the test file is placed at — so the id is read outside
// §2.3.1's lock and the record is written under it, and another `cr probe run`
// can land in between. §5.5.2 has a finding reference a probe by this id, so two
// records carrying one would make the reference ambiguous; the refusal is what
// keeps that impossible. Nothing is written, and the probe file has already
// been removed, so the experiment can simply be repeated.
//
// The mutation run of this task's gate is what asked for this test: nothing
// constructed an IDTakenError, so the branch and its message were unexercised.
func TestAReservedProbeIdAnotherRunTookIsRefused(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, gapProbeRunner, gapProbeTemplate)
	at := state.Stamp{Head: "be7e2c7", Round: 2}
	spent := &probe.Record{Kind: probe.Gap, Result: "passed", Target: "app.go:3", Baseline: "r1"}

	_, taken, err := recordProbe(
		prepared, fixtureOwner, fixtureProject, fixturePRNumber, at, nil, spent)
	require.NoError(t, err)
	require.Equal(t, "p1", taken, "the run that got there first spends the id")

	// The reservation this run made before its own placement, now stale.
	reserved := &probe.Record{
		ID: "p1", Kind: probe.Gap, Result: "failed", Target: "app.go:3", Baseline: "r1",
	}
	_, _, err = recordProbe(
		prepared, fixtureOwner, fixtureProject, fixturePRNumber, at, nil, reserved)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "probe id p1 was taken")
	assert.Contains(t, err.Error(), "the next free id is p2")
	assert.Equal(t, ExitState, exitCodeFor(err),
		"§11.2 codes a conflict between two cr runs 4, as it does the lock timeout")

	assert.Len(t, storedRecords(t, prepared, state.FileProbes), 1,
		"§5.5.2: nothing was written, so no two records carry the same id")
}

// Each kind takes the flags its own section names, and refuses the other's.
//
// Accepting and ignoring a flag would be the worse failure in every row here:
// the agent would have supplied an experiment, or named a line, and been told
// nothing about it. The absent runner log is what says the refusal came before
// §5.2.6 performed a baseline suite for an invocation cr was never going to
// carry out.
func TestEachProbeKindRefusesTheFlagsItDoesNotTake(t *testing.T) {
	supplied := writeProbeTest(t)
	patch := writePatch(t, fixtureDiff)

	for name, tc := range map[string]struct {
		args []string
		says string
	}{
		"a gap probe takes no patch": {
			args: []string{"--kind", "gap", "--test", supplied, "--target", "app.go:3",
				"--patch", patch},
			says: "--patch is rejected for a gap probe",
		},
		"a gap probe needs a test file": {
			args: []string{"--kind", "gap", "--target", "app.go:3"},
			says: "--test is required",
		},
		"a gap probe needs a target": {
			args: []string{"--kind", "gap", "--test", supplied},
			says: "--target is required for a gap probe",
		},
		"a mutation probe places no test file": {
			args: []string{"--kind", "mutation", "--patch", patch, "--test", supplied},
			says: "--test is rejected for a mutation probe",
		},
		"there are two kinds and no third": {
			args: []string{"--kind", "coverage", "--test", supplied},
			says: "§5.5 names two kinds of probe",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, _, log := probeFixture(t, gapProbeRunner, gapProbeTemplate)

			err := runCLI(t, append([]string{"probe", "run", fixturePR,
				"--repo", fixtureSlug}, tc.args...)...)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.says)
			assert.Equal(t, ExitUsage, exitCodeFor(err),
				"§11.2 codes a flag this kind does not take as a malformed invocation")
			assert.NoFileExists(t, log, "the refusal comes before any suite is run")
		})
	}
}

// §5.5: a gap probe's target is "supplied and validated as §6.2.3 validates a
// citation", against the head the round was briefed at.
//
// It is the row that decides whether the record can ever grade anything.
// §6.2.2 lets a `probed` finding rest on a probe only when the probe's target
// falls inside that finding's anchor range on the same path, so a target naming
// a file the head does not hold, or a line past the end of one it does, is a
// `path:line` cr would have written into a record and never opened. The
// experiment would have run, the sandbox would have been used, and the evidence
// chain would end at a location nobody can follow.
//
// Every case asserts the log is absent, which is the half a check placed after
// the run would fail: §5.2.6 performs a baseline suite before the probe, so a
// target validated at the write would already have cost two runs.
func TestAGapProbeRefusesATargetTheHeadDoesNotHold(t *testing.T) {
	for name, tc := range map[string]struct {
		target string
		says   string
	}{
		"a path the head does not hold": {
			target: "gone.go:1",
			says:   `names "gone.go", which the head under review does not hold as a file`,
		},
		"a line past the end of the file": {
			target: "app.go:9",
			says:   `names line 9 of "app.go", which holds 3 lines at the head under review`,
		},
		"a directory is not a file": {
			target: "tests:1",
			says:   "does not hold as a file",
		},
		"a path outside the repository": {
			target: "../../etc/passwd:1",
			says:   "does not hold as a file",
		},
		"not a path:line at all": {
			target: "app.go",
			says:   "is not a path:line",
		},
		"a line number cr would not have written": {
			target: "app.go:03",
			says:   `names line "03", which is not a line number`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, _, log := probeFixture(t, gapProbeRunner, gapProbeTemplate)
			supplied := writeProbeTest(t)

			err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
				"--kind", "gap", "--test", supplied, "--target", tc.target)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.says)
			assert.Contains(t, err.Error(), "§6.2.3",
				"§5.5 sends the reader to the rule the target is validated by")
			assert.Equal(t, ExitValidation, exitCodeFor(err),
				"§6.2.3 codes a location the head cannot resolve 1, as it does a citation's")
			assert.NoFileExists(t, log, "the refusal comes before any suite is run")
		})
	}
}

// §5.5's per-kind vocabulary at the boundary where a record is stored: a probe
// record whose result its kind does not admit never reaches probes.ndjson.
//
// The two crossed values are the whole reason §5.5 says the vocabulary "MUST
// NOT be shared". `passed` on a mutation probe reads as a suite that ran and
// noticed nothing, which is what §5.3.5 lets a `probed` grade rest on;
// `no-test-failed` on a gap probe reads as the missing test §5.4.5 says a gap
// probe never establishes. Either crossing over turns a question a colleague
// can answer into an assertion cr cannot support.
//
// The run record is what makes this a boundary test rather than a repeat of the
// package's own. A probe's own run carries the probe's id (§5.2.6), so the two
// records are written under one hold of §2.3.1's lock — and a check placed
// after the run's append would leave a run on disk naming a probe that was
// refused, which §5.2.6 would then have to read as a baseline it must not use.
func TestAProbeResultOutsideItsKindsVocabularyIsNeverStored(t *testing.T) {
	for name, tc := range map[string]struct {
		kind   probe.Kind
		result probe.Result
		says   string
	}{
		"passed is not a mutation result": {
			kind: probe.Mutation, result: "passed",
			says: `result "passed" is not one a mutation probe may carry`,
		},
		"no-test-failed is not a gap result": {
			kind: probe.Gap, result: "no-test-failed",
			says: `result "no-test-failed" is not one a gap probe may carry`,
		},
		"a kind §5.5 does not name has no vocabulary": {
			kind: "coverage", result: "failed",
			says: `probe kind "coverage" has no result vocabulary`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			prepared, _, _, _ := probeFixture(t, gapProbeRunner, gapProbeTemplate)
			at := state.Stamp{Head: "be7e2c7", Round: 2}
			measured := &run.Record{Filter: "Retry", OutputTail: "Tests:  1 passed\n"}

			_, _, err := recordProbe(
				prepared, fixtureOwner, fixtureProject, fixturePRNumber, at, measured,
				&probe.Record{
					Kind: tc.kind, Result: tc.result, Target: "app.go:3", Baseline: "r1",
				})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.says)
			assert.Contains(t, err.Error(), "§5.5")
			assert.Equal(t, ExitValidation, exitCodeFor(err),
				"§11.2 codes a record cr refuses to store 1")

			assert.Empty(t, storedRecords(t, prepared, state.FileProbes),
				"§5.5: the refused record never reached probes.ndjson")
			assert.Empty(t, storedRecords(t, prepared, state.FileRuns),
				"§5.2.6: nor did the run that would have named it")
		})
	}
}

// Every result its kind admits still reaches probes.ndjson, which is the half a
// check that refused everything would also pass.
func TestEveryResultItsKindAdmitsIsStored(t *testing.T) {
	for _, kind := range []probe.Kind{probe.Mutation, probe.Gap} {
		for _, result := range probe.Results(kind) {
			t.Run(string(kind)+"/"+string(result), func(t *testing.T) {
				prepared, _, _, _ := probeFixture(t, gapProbeRunner, gapProbeTemplate)

				_, id, err := recordProbe(
					prepared, fixtureOwner, fixtureProject, fixturePRNumber,
					state.Stamp{Head: "be7e2c7", Round: 2}, nil,
					&probe.Record{
						Kind: kind, Result: result, Target: "app.go:3", Baseline: "r1",
					})

				require.NoError(t, err)
				assert.Equal(t, "p1", id)
				stored := storedRecords(t, prepared, state.FileProbes)
				require.Len(t, stored, 1)
				assert.Equal(t, string(result), stored[0]["result"])
			})
		}
	}
}

// §5.5.1: probe records are immutable once written.
//
// The assertion is on the bytes, not on the count. A refusal that left one line
// on disk would satisfy a length check while having replaced it, and §5.5.1 is
// about the line a stored finding points at: §5.5.2 has a finding reference a
// probe by id, so a record rewritten under an id already spent would silently
// repoint every finding resting on it at a different experiment.
//
// The second write is a plausible one rather than a contrived one — a gap probe
// re-run after its first attempt, carrying the id §5.4.2 fixed before the
// placement and the result the second run produced. That is how §5.5.1 would be
// broken in practice, and the refusal names the id that is spent and the next
// one free, so the experiment can simply be repeated.
func TestAStoredProbeRecordIsNeverRewritten(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, gapProbeRunner, gapProbeTemplate)
	at := state.Stamp{Head: "be7e2c7", Round: 2}
	file := prepared.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileProbes)

	_, id, err := recordProbe(
		prepared, fixtureOwner, fixtureProject, fixturePRNumber, at, nil,
		&probe.Record{
			Kind: probe.Gap, Result: "passed", Target: "app.go:3", Baseline: "r1",
			Input: "it('cancels an expired hold', ...)",
		})
	require.NoError(t, err)
	require.Equal(t, "p1", id)
	written, err := os.ReadFile(file)
	require.NoError(t, err)

	_, _, err = recordProbe(
		prepared, fixtureOwner, fixtureProject, fixturePRNumber, at, nil,
		&probe.Record{
			ID: "p1", Kind: probe.Gap, Result: "failed", Target: "app.go:1", Baseline: "r2",
			Input: "it('cancels an expired hold', ...) // second attempt",
		})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "probe id p1 was taken")
	assert.Equal(t, ExitState, exitCodeFor(err))

	after, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, string(written), string(after),
		"§5.5.1: the stored record is byte for byte the one that was written")
}

// §5.5.3 through the reader that grades: a probe from an earlier head supports
// nothing in the current round, whatever its result was.
//
// `failed` is the case worth driving, because it is the one §5.4.4 would
// otherwise support — the other five results §5.4.5 settles at the run itself,
// so a head check that did nothing would be invisible in them. The record is
// stored at the round's own head and then read against a different one, which
// is what a force-push leaves behind: the probe file is still there, §5.5.1
// keeps it exactly as it was, and §5.5.3 is what removes its standing.
func TestAProbeFromAnEarlierHeadGradesNothing(t *testing.T) {
	stored := &probe.Record{
		ID: "p1", Kind: probe.Gap, Result: "failed", Target: "app.go:3", Baseline: "r1",
		Stamp: state.Stamp{Head: "be7e2c7", Round: 2},
	}

	assert.True(t, stored.Grades("be7e2c7"),
		"the head it ran against is the head it grades at")

	answered := answerGapSupport(
		stored,
		&state.Meta{Head: "9f2c1ab", Round: 3},
		[]run.Record{{ID: "r1", Passed: true, Stamp: state.Stamp{Head: "9f2c1ab"}}},
		nil,
		&finding.Finding{ID: "f1", Probe: "p1", Claim: "CR-1#c1", Unit: "u1"},
	)

	assert.False(t, answered.Supports,
		"§5.5.3: a probe from another head grades nothing in this round")
	assert.Contains(t, answered.Reason, "§5.5.3")
	assert.Contains(t, answered.Reason, "the probe ran at be7e2c7 and this round's head is 9f2c1ab")
}

// §5.6.4 end to end: `probe.max_per_round` caps probe executions per round, the
// cap being hit is reported, and the run past it executes nothing.
//
// Eleven probes at the built-in cap of ten, because the boundary is the whole
// rule. A cap asserted only from far below it would pass on an implementation
// that stopped at nine or at eleven, and either is a defect a reader could not
// see: one spends a probe the user budgeted for, the other runs one they did
// not.
//
// "Executes nothing" is asserted from the runner's own log rather than from the
// absence of a record. The runner appends to that log every time it starts, so
// a refusal placed after §5.2.6's baseline — or after `sandbox.Ensure` ran the
// profile's setup commands — would leave the log longer than it found it while
// probes.ndjson still held ten records, and a count would report that as a
// clean refusal.
//
// The tenth run is read through `--quiet`, because §11.1 exempts the cap report
// from it. §5.6.4's "never silently applied" is what that exemption is for: a
// reader who is told after the tenth probe that the round has no budget left
// learns it before the eleventh refuses, rather than from the refusal.
func TestTheEleventhProbeOfARoundIsRefusedHavingRunNothing(t *testing.T) {
	prepared, _, sandboxPath, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
	patch := writePatch(t, fixtureDiff)
	probing := []string{"probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", patch}

	// The default of §2.7's table, spelled here so a change to it fails
	// this test rather than passing under a cap nobody chose.
	const capped = 10
	for i := 1; i < capped; i++ {
		require.NoError(t, runCLI(t, probing...), "probe %d of the round's %d", i, capped)
	}

	filling := probeDocument(t, throughAPipe(t, append(probing, "--quiet")...))
	require.Equal(t, "p"+strconv.Itoa(capped), filling["probe"],
		"the tenth probe is the one that fills the round")
	honesty, ok := filling["honesty"].([]any)
	require.True(t, ok, "§11.1's disclosures are a field on the payload")
	require.Len(t, honesty, 1, "§5.6.4: the cap being hit is reported")
	assert.Contains(t, honesty[0], "§5.6.4")
	assert.Contains(t, honesty[0], "10 of 10 probes run this round")
	assert.Contains(t, honesty[0], "probe.max_per_round",
		"the report names the setting that would let the round run more")

	before, err := os.ReadFile(log)
	require.NoError(t, err)

	err = runCLI(t, probing...)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "10 of 10 probes run this round against probe.max_per_round")
	assert.Contains(t, err.Error(), "this run would be number 11")
	assert.Contains(t, err.Error(), "nothing was run")
	assert.Equal(t, ExitValidation, exitCodeFor(err),
		"§11.2 codes the round's spent budget 1, as it does §1.6.2's comment cap")

	after, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after),
		"§5.6.4: the refused run executed nothing in the sandbox")
	assert.Len(t, storedRecords(t, prepared, state.FileProbes), capped,
		"§5.6.4: the eleventh probe recorded nothing either")

	held, err := os.ReadFile(filepath.Join(sandboxPath, "app.go"))
	require.NoError(t, err)
	assert.Equal(t, fixtureSource, string(held),
		"the sandbox holds what the head holds, untouched by the run that was refused")
}

// A round with budget left is told nothing about the cap, and a round below it
// runs its probes.
//
// This is the half that makes the assertion above mean something: a cap that
// refused from the first probe, or a disclosure printed on every run, would
// satisfy the refusal and break §5.6.4 in the other direction — one stops cr
// probing at all, the other buries the one report that matters under nine that
// say nothing new.
func TestAProbeWithBudgetLeftIsToldNothingAboutTheCap(t *testing.T) {
	probeFixture(t, "echo 'Tests:  4 passed'\n")
	patch := writePatch(t, fixtureDiff)

	// The first run creates the sandbox, so §5.1.6's recreation notice is
	// there and the assertion is about the cap alone: no disclosure on
	// either run mentions §5.6.4, because neither hit it.
	first := runProbe(t, patch)
	require.Equal(t, "p1", first["probe"])
	assert.NotContains(t, disclosed(t, first), "§5.6.4",
		"§5.6.4: the cap is reported when it is hit, and the first of ten does not hit it")

	second := runProbe(t, patch)
	require.Equal(t, "p2", second["probe"], "the round's budget is not spent by one probe")
	assert.Equal(t, []any{}, second["honesty"],
		"the sandbox stood, so the second run has nothing to disclose at all")
}

// disclosed is a payload's §11.1 honesty entries joined into one string, for an
// assertion about what is not among them.
func disclosed(t *testing.T, payload map[string]any) string {
	t.Helper()
	entries, ok := payload["honesty"].([]any)
	require.True(t, ok, "§11.1's disclosures are a field on the payload")
	said := make([]string, 0, len(entries))
	for _, entry := range entries {
		text, isText := entry.(string)
		require.True(t, isText, "a disclosure is a sentence")
		said = append(said, text)
	}
	return strings.Join(said, "\n")
}
