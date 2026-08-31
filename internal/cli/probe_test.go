package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
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
