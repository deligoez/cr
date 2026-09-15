package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/run"
	"github.com/deligoez/cr/internal/state"
)

// §5.2.6 through the command: a gap probe whose freshly performed baseline does
// not stand, because §5.1.6's check failed after that run, is refused with
// §11.2's 3 and a hint naming the sandbox and `cr sandbox destroy`.
//
// QA found it refused with 2 and the malformed-invocation hint, while the
// message said to destroy the sandbox: two next steps that contradicted each
// other, and nothing about the command line was wrong. The runner here writes a
// file under the probe path template during the baseline, which is what §5.1.6
// calls a leftover artefact, so the baseline run is stored contaminated.
func TestAGapProbeWhoseBaselineRunLeftTheSandboxUncleanExitsThree(t *testing.T) {
	prepared, fixture, sandboxPath, _ := probeFixture(t,
		"if [ ! -f "+gapProbePath+" ]; then mkdir -p tests; echo stray > tests/cr_probe_zz.txt; fi\n"+
			"echo 'Tests:  5 passed'\n", gapProbeTemplate)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "gap", "--test", writeProbeTest(t), "--target", "app.go:3")

	var file *state.FileError
	require.True(t, errors.As(err, &file), "a failure of the sandbox the command used: %v", err)
	assert.Equal(t, "cannot use "+sandboxPath+": no baseline run stands at "+head+" for a gap probe: "+
		"the run performed for it was not one §5.2.6 admits; §5.1.6's check after that run found that "+
		"a probe artefact was left behind: tests/cr_probe_zz.txt", err.Error())
	assert.Equal(t, ExitFile, exitCodeFor(err), "§11.2: a file cr had to use and could not is 3")
	assert.Equal(t, "the baseline run left the sandbox "+sandboxPath+" unclean, so it measured nothing; "+
		"run `cr sandbox destroy 7 --repo "+fixtureSlug+"`, keep the suite from leaving that behind, "+
		"and run the probe again", hintFor(err))

	assert.Empty(t, storedRecords(t, prepared, state.FileProbes), "no probe was run or recorded")
	runs := storedRecords(t, prepared, state.FileRuns)
	require.Len(t, runs, 1)
	assert.Equal(t, true, runs[0]["contaminated"])
}

// §5.2.4 for a runner that could not be started: the run record says so and
// carries no exit status that reads as a run that succeeded.
//
// QA found such a run stored `exit_code: 0`. The runner is removed after the
// fixture is written, so both the baseline and the probe's own run fail to
// start, and both records are asserted.
func TestARunWhoseRunnerCouldNotStartIsStoredAsNotStarted(t *testing.T) {
	prepared, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n", gapProbeTemplate)
	require.NoError(t, os.Remove(filepath.Join(filepath.Dir(log), "runner.sh")))

	shown := runGap(t, writeProbeTest(t))
	assert.Equal(t, "error", shown["result"])

	runs := storedRecords(t, prepared, state.FileRuns)
	require.Len(t, runs, 2, "the baseline and the probe's own run")
	for _, stored := range runs {
		assert.Equal(t, true, stored["unstarted"], "the record says the runner did not start: %v", stored)
		assert.Equal(t, float64(-1), stored["exit_code"], "and holds no status that reads as success")
		assert.Equal(t, false, stored["passed"])
	}
}

// §5.1.7 through the command, with its reason: a probe voided by the post-run
// cleanliness check says so in its record and in its honesty, naming the
// tracked file that changed, and the next run's recreation notice names that
// cause rather than a baseline that is missing.
//
// QA found the probe carrying only `error`, and the next run saying "no
// post-setup baseline was recorded for it". The runner dirties a tracked file
// only while the mutation is applied, so the baseline stands and the probe is
// what is voided.
func TestAVoidedProbeNamesWhatTheCheckFoundHereAndInTheNextRecreation(t *testing.T) {
	prepared, _, sandboxPath, _ := probeFixture(t,
		onlyWhenMutated("  echo 'dirtied by the suite' >> .gitignore\n"))
	const found = "tracked files differ from the post-setup baseline: .gitignore"

	shown := runProbe(t, writePatch(t, fixtureDiff))
	reason := "§5.1.7: the post-run cleanliness check failed, so the result is error whatever the ladder " +
		"read, the probe grades no finding, and the sandbox is recreated before the next run: " + found
	assert.Equal(t, reason, shown["reason"])
	assert.Equal(t, []any{
		"sandbox " + sandboxPath + " recreated, per §5.1.6: there is no sandbox at that path",
		"probe p1 voided, per §5.1.7: " + found + "; its result is error, it grades no finding, " +
			"and the sandbox is recreated before the next run",
	}, shown["honesty"])
	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 1)
	assert.Equal(t, reason, probes[0]["reason"])

	var reported map[string]any
	next := afterHeader(t, throughAPipe(t, "test", fixturePR, "--repo", fixtureSlug))
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(next, "Tests:  4 passed\n")), &reported))
	assert.Equal(t, []any{
		"sandbox " + sandboxPath + " recreated, per §5.1.6: §5.1.7 forced its recreation: " +
			"probe p1 was voided after its run: " + found,
	}, reported["honesty"])
}

// mutatedHome is recordedHome's pull request with one mutation probe resting at
// the round's head, the baseline run it points at, and an empty mapping.
func mutatedHome(t *testing.T, result probe.Result, reason string, passed bool) {
	t.Helper()
	layout := recordedHome(t)
	stamp := state.Stamp{Head: recordHead, Round: recordRound}
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: recordOwner, Repo: recordRepo, PR: recordPRNum, IssueKey: gapIssue,
		Round: recordRound, Head: recordHead,
	}))
	require.NoError(t, held.Write(state.FileRuns, ndjson(t, run.Record{
		ID: "r1", Stamp: stamp, Passed: passed, OutputTail: "OK",
	})))
	require.NoError(t, held.Write(state.FileProbes, ndjson(t, probe.Record{
		ID: "p1", Stamp: stamp, Kind: probe.Mutation, Input: "--- a/x\n+++ b/x\n",
		Result: result, Reason: reason, Baseline: "r1",
		Target:     recordPath + ":" + strconv.Itoa(recordUnitStart[gapUnit]+3),
		OutputTail: "Tests:  4 passed",
	})))
	require.NoError(t, held.Write(state.FileMapping, ndjson[mapping.Pair](t)))
	require.NoError(t, held.Unlock())
}

// §5.3.5 through `cr record`: a record resting on a mutation probe is told what
// the probe supports and why, as a record resting on a gap probe is.
//
// QA found `probes` empty for a record resting on a voided mutation probe, so
// its demotion to argued went unexplained. A probe that recorded why it is
// `error` is named with that reason.
func TestRecordExplainsWhatAMutationProbeSupports(t *testing.T) {
	const voided = "§5.1.7: the post-run cleanliness check failed, so the result is error whatever the " +
		"ladder read, the probe grades no finding, and the sandbox is recreated before the next run: " +
		"tracked files differ from the post-setup baseline: src/Money.php"
	const unclean = "§5.3.4's sixth rung: the failed count is zero but the runner exited 255, so the run has " +
		"not said that nothing failed"
	for _, tc := range []struct {
		name     string
		result   probe.Result
		reason   string
		passed   bool
		supports bool
		grade    string
		said     string
	}{
		{
			name: "a voided probe", result: "error", reason: voided, passed: true, grade: "argued",
			said: "§5.3.5: only no-test-failed proves a gap, and a mutation probe whose result is error " +
				"supports no probed grade, so the record stays argued (§6.2) and is asked as a question " +
				"(§6.3); the probe recorded why it is error: " + voided,
		},
		{
			name: "an inconclusive probe", result: "inconclusive", reason: unclean, passed: true, grade: "argued",
			said: "§5.3.5: only no-test-failed proves a gap, and a mutation probe whose result is inconclusive " +
				"supports no probed grade, so the record stays argued (§6.2) and is asked as a question " +
				"(§6.3); the probe recorded why it is inconclusive: " + unclean,
		},
		{
			name: "a caught mutation", result: "failed", passed: true, grade: "argued",
			said: "§5.3.5: only no-test-failed proves a gap, and a mutation probe whose result is failed " +
				"supports no probed grade, so the record stays argued (§6.2) and is asked as a question (§6.3)",
		},
		{
			name: "a gap on a red suite", result: "no-test-failed", grade: "argued",
			said: "the baseline run r1 did not pass per §5.2.5, so §5.3.5 lets this no-test-failed prove " +
				"no gap and the record stays argued (§6.2)",
		},
		{
			name: "a gap on a passing baseline", result: "no-test-failed", passed: true, supports: true,
			grade: "probed",
			said: "the result is no-test-failed and the baseline run r1 passed per §5.2.5, so §5.3.5 lets " +
				"the record rest on this probe, for the tests the run selected only (§5.3.6)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutatedHome(t, tc.result, tc.reason, tc.passed)
			printed, err := runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson", aProbedRecord("")),
				"--repo", recordSlug)
			require.NoError(t, err)

			var payload struct {
				Recorded []map[string]any `json:"recorded"`
				Probes   []probeAnswer    `json:"probes"`
			}
			require.NoError(t, json.Unmarshal([]byte(printed), &payload))
			require.Len(t, payload.Recorded, 1)
			assert.Equal(t, tc.grade, payload.Recorded[0]["grade"], "the answer agrees with the grade")
			assert.Equal(t, []probeAnswer{{
				Record: "f1", Probe: "p1", Result: string(tc.result), Supports: tc.supports, Reason: tc.said,
			}}, payload.Probes)
		})
	}
}
