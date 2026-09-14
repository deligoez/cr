package cli

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §5.3.4's rung 6 and §5.4.3's rung 5 through `cr probe run`, with a runner
// that prints its recap and then exits non-zero.
//
// QA's S-S06-1: a runner printing `Tests: 3 ran` and then dying on a fatal
// error with exit 255 scored `no-test-failed`, established a gap, and graded a
// finding resting on it `probed` — while its own run record said `passed:
// false`. A zero failed count counts only on a clean exit, so both kinds now
// answer `inconclusive`, establish nothing, and the record resting on the
// probe stays `argued` and is asked as a question.
//
// Each kind carries its control, the same runner exiting 0, so the case is not
// passing on a fixture that could never reach the rung: the control has to
// reach `no-test-failed` and a `probed` grade for the mutation, and `passed`
// for the gap.
func TestAZeroFailedCountCountsOnlyOnACleanExit(t *testing.T) {
	mutation := func(t *testing.T) map[string]any { return runProbe(t, writePatch(t, fixtureDiff)) }
	gap := func(t *testing.T) map[string]any { return runGap(t, writeProbeTest(t)) }
	for _, tc := range []struct {
		name        string
		exit        int
		runner      func(then string) string
		run         func(t *testing.T) map[string]any
		result      string
		establishes string
		grade       string
		kind        string
		severity    string
	}{
		{
			name: "a mutation probe whose runner exited 255", exit: 255, runner: onlyWhenMutated, run: mutation,
			result: "inconclusive", establishes: establishesNothing,
			grade: "argued", kind: "question", severity: "high",
		},
		{
			name: "the same mutation probe exiting 0", exit: 0, runner: onlyWhenMutated, run: mutation,
			result: "no-test-failed", establishes: "gap",
			grade: "probed", kind: "finding", severity: "high",
		},
		{
			name: "a gap probe whose runner exited 255", exit: 255, runner: onlyWithTheProbeFile, run: gap,
			result: "inconclusive", establishes: establishesNothing,
			grade: "argued", kind: "question", severity: "medium",
		},
		{
			name: "the same gap probe exiting 0", exit: 0, runner: onlyWithTheProbeFile, run: gap,
			result: "passed", establishes: establishesBehaviour,
			grade: "argued", kind: "question", severity: "medium",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prepared, _, _, _ := probeFixture(t,
				tc.runner("  echo 'Tests:  4 passed'\n  exit "+strconv.Itoa(tc.exit)+"\n"), gapProbeTemplate)

			shown := tc.run(t)
			assert.Equal(t, tc.result, shown["result"])
			assert.Equal(t, tc.establishes, shown["establishes"])

			runs := storedRecords(t, prepared, state.FileRuns)
			require.Len(t, runs, 2, "the baseline and the probe run")
			assert.Equal(t, true, runs[0]["passed"], "the baseline exited 0 on the head's own code")
			assert.Equal(t,
				[]any{float64(tc.exit), float64(4), float64(0)},
				[]any{runs[1]["exit_code"], runs[1]["tests_run"], runs[1]["tests_failed"]},
				"the probe run printed four tests and no failure, whatever it exited with")

			recorded := recordOnTheProbe(t, prepared, tc.severity)
			assert.Equal(t, tc.grade, recorded["grade"])
			assert.Equal(t, tc.kind, recorded["kind"])
		})
	}
}

// §5.3.4's rung 5 and §5.4.3's rung 4 through `cr probe run`: a probe run whose
// output carries no count is `inconclusive` too, and its reason names that rung
// rather than an exit code, so the two `inconclusive` rungs are told apart.
func TestAnUndeterminedCountNamesItsRung(t *testing.T) {
	const undetermined = "the executed or failed count is undetermined, because no tests.count_pattern " +
		"is configured or the run's output did not yield the counts through it"
	for _, tc := range []struct {
		name   string
		runner string
		run    func(t *testing.T) map[string]any
		reason string
	}{
		{
			name:   "mutation",
			runner: onlyWhenMutated("  echo 'no recap'\n  exit 0\n"),
			run:    func(t *testing.T) map[string]any { return runProbe(t, writePatch(t, fixtureDiff)) },
			reason: "§5.3.4's fifth rung: " + undetermined,
		},
		{
			name:   "gap",
			runner: onlyWithTheProbeFile("  echo 'no recap'\n  exit 0\n"),
			run:    func(t *testing.T) map[string]any { return runGap(t, writeProbeTest(t)) },
			reason: "§5.4.3's fourth rung: " + undetermined,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prepared, _, _, _ := probeFixture(t, tc.runner, gapProbeTemplate)

			shown := tc.run(t)
			assert.Equal(t, "inconclusive", shown["result"])
			probes := storedRecords(t, prepared, state.FileProbes)
			require.Len(t, probes, 1)
			assertReason(t, tc.reason, shown, probes[0])

			runs := storedRecords(t, prepared, state.FileRuns)
			require.Len(t, runs, 2, "the baseline and the probe run")
			assert.Equal(t, []any{float64(0), nil, nil},
				[]any{runs[1]["exit_code"], runs[1]["tests_run"], runs[1]["tests_failed"]},
				"the probe run exited 0 and its output determined no count")
		})
	}
}

// assertReason holds the command's document and the stored probe record to the
// same reason, and to none at all where want is empty.
func assertReason(t *testing.T, want string, shown, stored map[string]any) {
	t.Helper()
	if want == "" {
		assert.NotContains(t, shown, "reason", "the document carries no reason")
		assert.NotContains(t, stored, "reason", "the record carries no reason")
		return
	}
	assert.Equal(t, want, shown["reason"])
	assert.Equal(t, want, stored["reason"])
}

// recordOnTheProbe runs `cr record` over one test-adequacy record resting on
// probe p1, anchored on the line the probes target, and returns the record as
// cr stored it.
//
// The unit is written into the probe fixture's round rather than formed by
// `cr brief`, because what is under test is the grade §6.2 computes from the
// probe `cr probe run` stored, and the unit only has to hold the anchor.
func recordOnTheProbe(t *testing.T, prepared state.Layout, severity string) map[string]any {
	t.Helper()
	meta, err := prepared.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileUnits, fmt.Appendf(nil,
		`{"id":"u1","path":"app.go","hunk_ranges":[{"start":1,"end":3}],"head":%q,"round":%d}`+"\n",
		meta.Head, meta.Round)))
	require.NoError(t, held.Unlock())

	record := map[string]any{
		"id": "f1", "kind": "finding", "role": "test-adequacy", "class": "untested-retry",
		"severity": severity, "unit": "u1", "probe": "p1",
		"anchor": map[string]any{
			"path": "app.go", "side": "RIGHT", "start_line": 3, "line": 3,
			"content_hash": "0123456789abcdef",
		},
		"summary":  "No test notices Retry losing its backoff.",
		"evidence": "The mutation removing the call left the suite green.",
	}
	_, err = runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", record), "--repo", fixtureSlug)
	require.NoError(t, err)

	stored := storedRecords(t, prepared, state.FileFindings)
	require.Len(t, stored, 1)
	return stored[0]
}
