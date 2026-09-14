package probe

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gapRung is one step of §5.4.3's ladder, restated as a standalone predicate
// over the whole of GapMeasured rather than as one arm of a switch that only
// sees the runs the arms above it let through.
//
// The restatement is what makes the section's claim checkable. §5.4.3 declares
// its ladder "total over every run", and a table that drove six inputs through
// GapLadder and compared six answers would prove neither half of that: a caller
// of GapLadder only ever sees the first rung that fires, so a second one firing
// on the same run is invisible, and a run that fired nothing would still be
// answered by the final `return`. Asking every rung about every run, and
// requiring exactly one to say yes, is the claim itself rather than a sample of
// its consequences.
//
// Where a predicate names an earlier rung's condition it is §5.4.3's own order
// spelled out, not the implementation copied. The order does real work twice: a
// run killed for the timeout has usually also died on a signal, so rungs 1 and
// 2 would both answer it, and a run that selected no test at all reports zero
// failures, so rung 5 — "the failed count is zero" — would answer it beside
// rung 3. That second collision is the one §5.4.5 turns on, since rung 5's
// `passed` caps severity at `medium` and rung 3's `no-tests-selected` supports
// nothing at all.
type gapRung struct {
	// name is the section's own wording for this rung.
	name string
	// matches is the rung's condition, read as §5.4.3's order places it.
	matches func(GapMeasured) bool
	// result is what the rung answers with.
	result Result
}

// gapRan reports that the run reached the point where its counts mean anything:
// the runner started, and it finished by returning rather than by being killed.
// It is the negation of rungs 1 and 2 together, which is why the four count
// rungs share it and none of them repeats it.
func gapRan(m GapMeasured) bool {
	return !m.TimedOut && !m.Unstarted && m.ExitCode >= 0
}

// gapRungs is §5.4.3's ladder, in the section's order.
//
// The four count rungs partition the two counts by their own text once rung 3
// has taken the known-zero executed count away: undetermined, known zero, known
// non-zero with no failures, known non-zero with failures. Nothing else is
// representable, so nothing falls between them.
var gapRungs = []gapRung{
	{
		name:    "1 · the run was killed for exceeding tests.timeout_seconds",
		matches: func(m GapMeasured) bool { return m.TimedOut },
		result:  resultTimeout,
	},
	{
		name: "2 · the runner could not be started, or exited on a signal",
		matches: func(m GapMeasured) bool {
			return !m.TimedOut && (m.Unstarted || m.ExitCode < 0)
		},
		result: ResultError,
	},
	{
		name: "3 · the executed test count is known and equals zero",
		matches: func(m GapMeasured) bool {
			return gapRan(m) && m.TestsRun != nil && *m.TestsRun == 0
		},
		result: resultNoTestsSelected,
	},
	{
		name: "4 · the executed or failed count is undetermined",
		matches: func(m GapMeasured) bool {
			return gapRan(m) &&
				(m.TestsRun == nil || (*m.TestsRun != 0 && m.TestsFailed == nil))
		},
		result: resultInconclusive,
	},
	{
		name: "5 · the failed count is zero, and the run exited 0",
		matches: func(m GapMeasured) bool {
			return gapRan(m) && m.TestsRun != nil && *m.TestsRun != 0 &&
				m.TestsFailed != nil && *m.TestsFailed == 0 && m.ExitCode == 0
		},
		result: resultPassed,
	},
	{
		name: "5 · the failed count is zero, and the run did not exit 0",
		matches: func(m GapMeasured) bool {
			return gapRan(m) && m.TestsRun != nil && *m.TestsRun != 0 &&
				m.TestsFailed != nil && *m.TestsFailed == 0 && m.ExitCode != 0
		},
		result: resultInconclusive,
	},
	{
		name: "6 · otherwise",
		matches: func(m GapMeasured) bool {
			return gapRan(m) && m.TestsRun != nil && *m.TestsRun != 0 &&
				m.TestsFailed != nil && *m.TestsFailed != 0
		},
		result: resultFailed,
	},
}

// onlyGapRung returns the one rung matching m, failing the test when a second
// rung matches it or when none does. Both failures are §5.4.3's totality claim,
// and the message says which half broke.
func onlyGapRung(t *testing.T, m GapMeasured) gapRung {
	t.Helper()
	matched := make([]string, 0, len(gapRungs))
	var fired gapRung
	for _, candidate := range gapRungs {
		if candidate.matches(m) {
			matched = append(matched, candidate.name)
			fired = candidate
		}
	}
	require.Len(t, matched, 1,
		"§5.4.3: the ladder is total over every run; %s matched %v", describeGap(m), matched)
	return fired
}

// describeGap names a GapMeasured the way the failure message needs it, with
// the two pointers read rather than printed as addresses: the difference
// between a known zero and no count at all is the whole of rungs 3 and 4.
func describeGap(m GapMeasured) string {
	return fmt.Sprintf(
		"{timed_out:%t unstarted:%t exit:%d tests_run:%s tests_failed:%s}",
		m.TimedOut, m.Unstarted, m.ExitCode, counted(m.TestsRun), counted(m.TestsFailed))
}

// §5.4.3's six rungs, each driven by a run that reaches it, and each checked
// against the whole ladder rather than against its own arm alone.
//
// The cases that carry the most are the ones where a lower rung would also have
// answered. A run killed for the timeout also exited on a signal, and rung 1
// takes it from rung 2. A crashed runner that printed a count rung 6 could read
// is rung 2's, which is round 9's probe-ladder-asymmetry finding and the reason
// the rung reads the exit code at all: §5.4.4 makes a supported failed gap
// probe carry severity at least `high`, so without it a segmentation fault
// becomes the loudest item in the draft. A filter that selected nothing reports
// zero failures, and rung 3 takes it from rung 5 — the difference between a
// probe that establishes nothing and one §5.4.5 caps at `medium`. And a failed
// count of zero beside an undetermined executed count is rung 4's, not rung
// 5's: a runner that printed nothing cr could parse has not said that nothing
// failed.
func TestTheGapLadderAnswersEveryRunWithExactlyOneRung(t *testing.T) {
	for _, tc := range []struct {
		name     string
		measured GapMeasured
		result   Result
		rung     string
	}{
		{
			name:     "a run killed for the budget is a timeout, not the signal that killed it",
			measured: GapMeasured{TimedOut: true, ExitCode: -9},
			result:   resultTimeout,
			rung:     gapRungs[0].name,
		},
		{
			name:     "a timeout outranks every reading of the counts",
			measured: GapMeasured{TimedOut: true, TestsRun: new(12), TestsFailed: new(3)},
			result:   resultTimeout,
			rung:     gapRungs[0].name,
		},
		{
			name:     "a runner that could not be started",
			measured: GapMeasured{Unstarted: true},
			result:   ResultError,
			rung:     gapRungs[1].name,
		},
		{
			name:     "a runner that crashed after printing a count the ladder could read",
			measured: GapMeasured{ExitCode: -11, TestsRun: new(4), TestsFailed: new(1)},
			result:   ResultError,
			rung:     gapRungs[1].name,
		},
		{
			name:     "a filter that selected nothing reports no failures and is still empty",
			measured: GapMeasured{TestsRun: new(0), TestsFailed: new(0)},
			result:   resultNoTestsSelected,
			rung:     gapRungs[2].name,
		},
		{
			name:     "a known executed count of zero, with no failed count at all",
			measured: GapMeasured{TestsRun: new(0)},
			result:   resultNoTestsSelected,
			rung:     gapRungs[2].name,
		},
		{
			name:     "no tests.count_pattern configured, so neither count is derivable",
			measured: GapMeasured{ExitCode: 1},
			result:   resultInconclusive,
			rung:     gapRungs[3].name,
		},
		{
			name:     "an executed count the runner printed, with no failed count",
			measured: GapMeasured{TestsRun: new(12)},
			result:   resultInconclusive,
			rung:     gapRungs[3].name,
		},
		{
			name:     "a failed count of zero is worth nothing while the executed count is undetermined",
			measured: GapMeasured{TestsFailed: new(0)},
			result:   resultInconclusive,
			rung:     gapRungs[3].name,
		},
		{
			name:     "the supplied test ran and nothing failed, so the behaviour is present",
			measured: GapMeasured{TestsRun: new(5), TestsFailed: new(0)},
			result:   resultPassed,
			rung:     gapRungs[4].name,
		},
		{
			name:     "a runner that printed its count and then exited non-zero has not said nothing failed",
			measured: GapMeasured{ExitCode: 255, TestsRun: new(5), TestsFailed: new(0)},
			result:   resultInconclusive,
			rung:     gapRungs[5].name,
		},
		{
			name:     "the supplied test ran and failed",
			measured: GapMeasured{TestsRun: new(5), TestsFailed: new(1)},
			result:   resultFailed,
			rung:     gapRungs[6].name,
		},
		{
			name:     "the supplied test failed and the runner exited non-zero, as runners do",
			measured: GapMeasured{ExitCode: 1, TestsRun: new(5), TestsFailed: new(1)},
			result:   resultFailed,
			rung:     gapRungs[6].name,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fired := onlyGapRung(t, tc.measured)
			assert.Equal(t, tc.rung, fired.name,
				"§5.4.3: the rung the section puts this run on")
			assert.Equal(t, tc.result, GapLadder(tc.measured),
				"§5.4.3: the result is the first matching rung's")
		})
	}
}

// §5.4.3's totality over the whole input rather than over eleven chosen runs.
//
// Every combination of the fields the ladder reads is driven through, with the
// counts spanning the three states the pointer exists to tell apart: no count,
// a known zero, and a known non-zero. Exactly one rung must answer each, and
// GapLadder must return that rung's result — so a rung condition widened by one
// boundary, or an order swapped by one place, has nowhere to hide.
func TestEveryRunTheGapLadderCanSeeMatchesOneRung(t *testing.T) {
	counts := []*int{nil, new(0), new(7)}
	for _, timedOut := range []bool{false, true} {
		for _, unstarted := range []bool{false, true} {
			for _, code := range []int{-9, 0, 1} {
				for _, executed := range counts {
					for _, failed := range counts {
						m := GapMeasured{
							TimedOut:    timedOut,
							Unstarted:   unstarted,
							ExitCode:    code,
							TestsRun:    executed,
							TestsFailed: failed,
						}
						t.Run(describeGap(m), func(t *testing.T) {
							assert.Equal(t, onlyGapRung(t, m).result, GapLadder(m),
								"§5.4.3: the ladder answers with the one rung that matched")
						})
					}
				}
			}
		}
	}
}

// The two ladders never answer with each other's vocabulary.
//
// §5.5's second table is explicit that the result vocabulary "is per kind and
// MUST NOT be shared", and the two ladders are where a value is chosen. The
// pair that matters is `no-test-failed` and `passed`: they sit on the same rung
// of the same shape and mean opposite things — §5.3.5 lets one prove a test gap
// and §5.4.5 says the other establishes only that the behaviour is present —
// so a ladder that borrowed the other's value would license an assertion the
// experiment did not support.
func TestNeitherLadderProducesTheOtherKindsResult(t *testing.T) {
	counts := []*int{nil, new(0), new(7)}
	for _, code := range []int{-9, 0, 1} {
		for _, executed := range counts {
			for _, failed := range counts {
				for _, flags := range [][2]bool{{false, false}, {true, false}, {false, true}} {
					gap := GapLadder(GapMeasured{
						TimedOut: flags[0], Unstarted: flags[1], ExitCode: code,
						TestsRun: executed, TestsFailed: failed,
					})
					assert.NotEqual(t, resultNoTestFailed, gap,
						"§5.5: no-test-failed is the mutation vocabulary's, and §5.3.5 alone "+
							"lets it prove a test gap")

					mutation := Ladder(Measured{
						Applied: true, TimedOut: flags[0], Unstarted: flags[1], ExitCode: code,
						TestsRun: executed, TestsFailed: failed,
					})
					assert.NotEqual(t, resultPassed, mutation,
						"§5.5: passed is the gap vocabulary's, and §5.4.5 caps what it supports")
				}
			}
		}
	}
}
