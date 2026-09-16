package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/run"
)

// §5.3.5 and §5.3.7: what one mutation probe's result may be read for.
//
// Every value §5.3.4's ladder can produce is driven against a baseline that
// passed and one that did not, so the table says what is refused as plainly as
// what is allowed. §5.3.5 names four results by hand — `timeout`, `error`,
// `no-tests-selected` and `inconclusive` — and none of them supports a `probed`
// grade under either baseline, which is the empty selection, the unparseable
// runner and the run that never finished all failing to manufacture evidence.
//
// The row that matters most is `no-test-failed` against a failing baseline. It
// is the result §5.3.5 does let prove a gap, and a repository whose suite is
// already red produces it for every mutation anyone tries — so the baseline
// condition is the whole difference between an experiment and a coincidence.
func TestOnlyNoTestFailedAgainstAPassingBaselineProvesTheGap(t *testing.T) {
	for _, tc := range []struct {
		result    Result
		proves    bool
		disproves bool
	}{
		{result: ResultError},
		{result: resultTimeout},
		{result: resultNoTestsSelected},
		{result: resultInconclusive},
		{result: resultNoTestFailed, proves: true},
		{result: resultFailed, disproves: true},
	} {
		t.Run(string(tc.result), func(t *testing.T) {
			outcome := Decide(tc.result, "")

			assert.Equal(t, tc.proves, Proves(outcome, resolvedBaseline(t, true)),
				"§5.3.5: only no-test-failed proves the gap")
			assert.False(t, Proves(outcome, resolvedBaseline(t, false)),
				"§5.3.5: and only when the baseline run record has passed: true")
			assert.Equal(t, tc.disproves, Disproves(outcome),
				"§5.3.7: only failed disproves it")
		})
	}
}

// §5.1.7: a probe whose sandbox failed the post-run check "establishes nothing
// in either direction", and the two directions are §5.3.5's and §5.3.7's.
//
// Neither is read off a value the ladder produced, because the value the record
// carries is not that one: Decide is Outcome's only constructor and it wrote
// `error` over both. The assertion is here rather than left to outcome_test.go
// because that file proves what the record says, and this is what the record is
// then allowed to support.
func TestAVoidedProbeNeitherProvesNorDisprovesTheGap(t *testing.T) {
	const unclean = "a probe artefact was left behind: cr_probe_p1.txt"

	assert.False(t, Proves(Decide(resultNoTestFailed, unclean), resolvedBaseline(t, true)),
		"§5.1.7 overrode the one result §5.3.5 lets prove a gap")
	assert.False(t, Disproves(Decide(resultFailed, unclean)),
		"§5.1.7 overrode the one result §5.3.7 lets disprove it")
}

// resolvedBaseline is the baseline a probe carries, built the only way one can
// be: §5.2.6 resolves it out of a stored run record, and §5.2.5's verdict comes
// with it. Proves reads that verdict through an unexported field, so a caller
// cannot hand it a passing baseline the runs never recorded.
func resolvedBaseline(t *testing.T, passed bool) Baseline {
	t.Helper()
	const head = "0a1b2c3"
	record := baselineRun(head, "", nil)
	record.ID = "r1"
	record.Passed = passed
	baseline, resolved := Spec{}.Resolve([]run.Record{record}, head)
	require.True(t, resolved, "the fixture is a run record §5.2.6 admits")
	return baseline
}
