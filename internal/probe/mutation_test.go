package probe

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rung is one step of §5.3.4's ladder, restated as a standalone predicate over
// the whole of Measured rather than as one arm of a switch that only sees the
// runs the arms above it let through.
//
// The restatement is what makes the section's claim checkable. §5.3.4 declares
// its ladder "total over every run — no run can match two rungs, and none can
// match none", and a table that drove seven inputs through Ladder and compared
// seven answers would prove neither half: a caller of Ladder only ever sees the
// first rung that fires, so a second one firing on the same run is invisible,
// and a run that fired nothing would still be answered by the final `return`.
// Asking every rung about every run, and requiring exactly one to say yes, is
// the claim itself rather than a sample of its consequences.
//
// Where a predicate names an earlier rung's condition it is §5.3.4's own order
// spelled out, not the implementation copied. The order does real work twice: a
// run killed for the timeout has usually also died on a signal, so rungs 2 and
// 3 would both answer it, and a run that selected no test at all reports zero
// failures, so rung 6 — "the failed count is zero" — would answer it beside
// rung 4. That second collision is the one §5.3.5 turns on, since rung 6 is the
// only result that may prove a gap and rung 4's may not.
type rung struct {
	// name is the section's own wording for this rung.
	name string
	// matches is the rung's condition, read as §5.3.4's order places it.
	matches func(Measured) bool
	// result is what the rung answers with.
	result Result
}

// ran reports that the run reached the point where its counts mean anything:
// the patch applied, the runner started, and it finished by returning rather
// than by being killed. It is the negation of rungs 1 to 3 together, which is
// why the four count rungs share it and none of them repeats it.
func ran(m Measured) bool {
	return m.Applied && !m.TimedOut && !m.Unstarted && m.ExitCode >= 0
}

// rungs is §5.3.4's ladder, in the section's order.
//
// The four count rungs partition the two counts by their own text once rung 4
// has taken the known-zero executed count away: undetermined, known zero,
// known non-zero with no failures, known non-zero with failures. Nothing else
// is representable, so nothing falls between them.
var rungs = []rung{
	{
		name:    "1 · the mutation patch did not apply cleanly",
		matches: func(m Measured) bool { return !m.Applied },
		result:  ResultError,
	},
	{
		name:    "2 · the run was killed for exceeding tests.timeout_seconds",
		matches: func(m Measured) bool { return m.Applied && m.TimedOut },
		result:  resultTimeout,
	},
	{
		name: "3 · the runner could not be started, or exited on a signal",
		matches: func(m Measured) bool {
			return m.Applied && !m.TimedOut && (m.Unstarted || m.ExitCode < 0)
		},
		result: ResultError,
	},
	{
		name: "4 · the executed test count is known and equals zero",
		matches: func(m Measured) bool {
			return ran(m) && m.TestsRun != nil && *m.TestsRun == 0
		},
		result: resultNoTestsSelected,
	},
	{
		name: "5 · the executed or failed count is undetermined",
		matches: func(m Measured) bool {
			return ran(m) &&
				(m.TestsRun == nil || (*m.TestsRun != 0 && m.TestsFailed == nil))
		},
		result: resultInconclusive,
	},
	{
		name: "6 · the failed count is zero, and the run exited 0",
		matches: func(m Measured) bool {
			return ran(m) && m.TestsRun != nil && *m.TestsRun != 0 &&
				m.TestsFailed != nil && *m.TestsFailed == 0 && m.ExitCode == 0
		},
		result: resultNoTestFailed,
	},
	{
		name: "6 · the failed count is zero, and the run did not exit 0",
		matches: func(m Measured) bool {
			return ran(m) && m.TestsRun != nil && *m.TestsRun != 0 &&
				m.TestsFailed != nil && *m.TestsFailed == 0 && m.ExitCode != 0
		},
		result: resultInconclusive,
	},
	{
		name: "7 · otherwise",
		matches: func(m Measured) bool {
			return ran(m) && m.TestsRun != nil && *m.TestsRun != 0 &&
				m.TestsFailed != nil && *m.TestsFailed != 0
		},
		result: resultFailed,
	},
}

// only returns the one rung matching m, failing the test when a second rung
// matches it or when none does. Both failures are §5.3.4's totality claim, and
// the message says which half broke.
func only(t *testing.T, m Measured) rung {
	t.Helper()
	matched := make([]string, 0, len(rungs))
	var fired rung
	for _, candidate := range rungs {
		if candidate.matches(m) {
			matched = append(matched, candidate.name)
			fired = candidate
		}
	}
	require.Len(t, matched, 1,
		"§5.3.4: no run can match two rungs, and none can match none; %s matched %v",
		describe(m), matched)
	return fired
}

// describe names a Measured the way the failure message needs it, with the two
// pointers read rather than printed as addresses: the difference between a
// known zero and no count at all is the whole of rungs 4 and 5.
func describe(m Measured) string {
	return fmt.Sprintf(
		"{applied:%t timed_out:%t unstarted:%t exit:%d tests_run:%s tests_failed:%s}",
		m.Applied, m.TimedOut, m.Unstarted, m.ExitCode,
		counted(m.TestsRun), counted(m.TestsFailed))
}

// counted prints a count, or "undetermined" when there is none.
func counted(n *int) string {
	if n == nil {
		return "undetermined"
	}
	return fmt.Sprintf("%d", *n)
}

// §5.3.4's seven rungs, each driven by a run that reaches it, and each checked
// against the whole ladder rather than against its own arm alone.
//
// The cases that carry the most are the ones where a lower rung would also have
// answered. A run killed for the timeout also exited on a signal, and rung 2
// takes it from rung 3. A filter that selected nothing reports zero failures,
// and rung 4 takes it from rung 6 — which is the difference between a probe
// that §5.3.5 refuses to grade on and one it lets prove a gap. And a failed
// count of zero beside an undetermined executed count is rung 5's, not rung
// 6's: a runner that printed nothing cr could parse has not said that nothing
// failed.
func TestTheLadderAnswersEveryRunWithExactlyOneRung(t *testing.T) {
	for _, tc := range []struct {
		name     string
		measured Measured
		result   Result
		rung     string
	}{
		{
			name:     "the patch did not apply, so no tests were run",
			measured: Measured{},
			result:   ResultError,
			rung:     rungs[0].name,
		},
		{
			name: "a patch that did not apply outranks everything after it",
			measured: Measured{
				TimedOut:    true,
				Unstarted:   true,
				ExitCode:    -9,
				TestsRun:    new(12),
				TestsFailed: new(3),
			},
			result: ResultError,
			rung:   rungs[0].name,
		},
		{
			name:     "a run killed for the budget is a timeout, not the signal that killed it",
			measured: Measured{Applied: true, TimedOut: true, ExitCode: -9},
			result:   resultTimeout,
			rung:     rungs[1].name,
		},
		{
			name:     "a runner that could not be started",
			measured: Measured{Applied: true, Unstarted: true},
			result:   ResultError,
			rung:     rungs[2].name,
		},
		{
			name:     "a runner that exited on a signal",
			measured: Measured{Applied: true, ExitCode: -11, TestsRun: new(4)},
			result:   ResultError,
			rung:     rungs[2].name,
		},
		{
			name: "a filter that selected nothing reports no failures and is still empty",
			measured: Measured{
				Applied: true, TestsRun: new(0), TestsFailed: new(0),
			},
			result: resultNoTestsSelected,
			rung:   rungs[3].name,
		},
		{
			name:     "a known executed count of zero, with no failed count at all",
			measured: Measured{Applied: true, TestsRun: new(0)},
			result:   resultNoTestsSelected,
			rung:     rungs[3].name,
		},
		{
			name:     "no tests.count_pattern configured, so neither count is derivable",
			measured: Measured{Applied: true, ExitCode: 1},
			result:   resultInconclusive,
			rung:     rungs[4].name,
		},
		{
			name:     "an executed count the runner printed, with no failed count",
			measured: Measured{Applied: true, TestsRun: new(12)},
			result:   resultInconclusive,
			rung:     rungs[4].name,
		},
		{
			name:     "a failed count of zero is worth nothing while the executed count is undetermined",
			measured: Measured{Applied: true, TestsFailed: new(0)},
			result:   resultInconclusive,
			rung:     rungs[4].name,
		},
		{
			name: "the suite ran and nothing failed, which is the only rung that proves a gap",
			measured: Measured{
				Applied: true, TestsRun: new(12), TestsFailed: new(0),
			},
			result: resultNoTestFailed,
			rung:   rungs[5].name,
		},
		{
			name: "a runner that printed its count and then exited non-zero has not said nothing failed",
			measured: Measured{
				Applied: true, ExitCode: 255, TestsRun: new(3), TestsFailed: new(0),
			},
			result: resultInconclusive,
			rung:   rungs[6].name,
		},
		{
			name: "a test caught the mutation",
			measured: Measured{
				Applied: true, TestsRun: new(12), TestsFailed: new(1),
			},
			result: resultFailed,
			rung:   rungs[7].name,
		},
		{
			name: "a test caught the mutation and the runner exited non-zero, as runners do",
			measured: Measured{
				Applied: true, ExitCode: 1, TestsRun: new(12), TestsFailed: new(1),
			},
			result: resultFailed,
			rung:   rungs[7].name,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fired := only(t, tc.measured)
			assert.Equal(t, tc.rung, fired.name,
				"§5.3.4: the rung the section puts this run on")
			assert.Equal(t, tc.result, Ladder(tc.measured),
				"§5.3.4: the result is the first matching rung's")
		})
	}
}

// §5.3.4's totality over the whole input rather than over twelve chosen runs.
//
// Every combination of the fields the ladder reads is driven through, with the
// counts spanning the three states the pointer exists to tell apart: no count,
// a known zero, and a known non-zero. Exactly one rung must answer each, and
// Ladder must return that rung's result — so a rung condition widened by one
// boundary, or an order swapped by one place, has nowhere to hide.
func TestEveryRunTheLadderCanSeeMatchesOneRung(t *testing.T) {
	counts := []*int{nil, new(0), new(7)}
	for _, applied := range []bool{false, true} {
		for _, timedOut := range []bool{false, true} {
			for _, unstarted := range []bool{false, true} {
				for _, code := range []int{-9, 0, 1} {
					for _, executed := range counts {
						for _, failed := range counts {
							m := Measured{
								Applied:     applied,
								TimedOut:    timedOut,
								Unstarted:   unstarted,
								ExitCode:    code,
								TestsRun:    executed,
								TestsFailed: failed,
							}
							t.Run(describe(m), func(t *testing.T) {
								assert.Equal(t, only(t, m).result, Ladder(m),
									"§5.3.4: the ladder answers with the one rung that matched")
							})
						}
					}
				}
			}
		}
	}
}
