package probe

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/run"
)

// §5.2.6: when no matching run exists at the current head, cr performs and
// records the baseline before the probe.
//
// The order of what gets performed is asserted as well as the count, because
// §5.2.2 requires the unfiltered run of every probe and the filtered one only
// additionally — a mutation probe that performed its filtered run alone would
// have measured its own narrow slice and learned nothing about the failures the
// rest of the suite was already carrying.
func TestAnAbsentBaselineIsPerformedBeforeTheProbe(t *testing.T) {
	const head = "0a1b2c3"
	const filter = "handles an empty cart"

	for _, tc := range []struct {
		name      string
		kind      Kind
		performed []Spec
		resolved  string
	}{
		{
			name:      "a filtered mutation probe performs both and points at the filtered run",
			kind:      Mutation,
			performed: []Spec{{}, {Filter: filter}},
			resolved:  "r2",
		},
		{
			name:      "a filtered gap probe performs the unfiltered run and points at it",
			kind:      Gap,
			performed: []Spec{{}},
			resolved:  "r1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var performed []Spec
			resolved, err := Ensure(nil, head, tc.kind, filter,
				recordingPerformer(head, &performed))
			require.NoError(t, err)
			assert.Equal(t, tc.performed, performed,
				"§5.2.2: the unfiltered run first, the filtered one additionally")
			assert.Equal(t, tc.resolved, resolved.ID(),
				"§5.5: the baseline column names the run for this kind")
			assert.True(t, resolved.Passed(),
				"§5.2.5's verdict travels out of the record that was chosen")
		})
	}
}

// recordingPerformer stands in for the run `cr test` performs, noting which
// baselines it was asked for and handing back a record that stands as one.
func recordingPerformer(head string, into *[]Spec) Performer {
	return func(spec Spec) (run.Record, error) {
		*into = append(*into, spec)
		performed := baselineRun(head, spec.Filter)
		performed.ID = "r" + strconv.Itoa(len(*into))
		performed.Passed = true
		return performed, nil
	}
}

// §5.2.6: "When more than one matches, the most recent MUST be used."
//
// The two candidates here disagree about §5.2.5's verdict, so the assertion
// says which record was chosen rather than only which id came back. A resolver
// that took the first match would hand §5.3.5 a `passed: false` it could refuse
// the probe on, from a run the head had already superseded.
func TestTwoBaselineCandidatesResolveToTheMostRecent(t *testing.T) {
	const head = "0a1b2c3"

	superseded := baselineRun(head, "")
	superseded.ID = "r1"
	superseded.Passed = false
	newest := baselineRun(head, "")
	newest.ID = "r4"
	newest.Passed = true

	resolved, err := Ensure(
		[]run.Record{superseded, newest}, head, Gap, "", neverPerformed(t))
	require.NoError(t, err)
	assert.Equal(t, "r4", resolved.ID(), "§5.2.6: the most recent match is used")
	assert.True(t, resolved.Passed(), "the verdict is the chosen record's own")
}

// neverPerformed is a performer that fails the test if it is called, which is
// how "a matching run exists at the current head" is asserted: §5.2.6 performs
// a baseline only when none does.
func neverPerformed(t *testing.T) Performer {
	t.Helper()
	return func(spec Spec) (run.Record, error) {
		t.Errorf("§5.2.6 performed a baseline for %+v that was already on file", spec)
		return run.Record{}, nil
	}
}

// §5.2.6: "Only a run record carrying no `probe` may serve as a baseline."
//
// The fence is structural rather than checked — Resolve is Baseline's only
// constructor and it never builds one out of a record carrying a probe — so
// what is asserted here is the consequence: a probe's own mutated or
// probe-injected run is passed over, and §5.2.6 performs a fresh one instead.
// The second case is the one the fence would otherwise strand: a first probe at
// a head nothing has been run at still obtains a baseline, because the run
// §5.2.6 performs is on un-probed code by construction.
func TestAProbesOwnRunNeverBecomesTheNextProbesBaseline(t *testing.T) {
	const head = "0a1b2c3"

	// Ids well clear of the one recordingPerformer allocates, so the
	// assertion below names the run that was performed and not a record
	// that merely shares its id.
	mutated := baselineRun(head, "", probed)
	mutated.ID = "r7"
	mutated.Passed = true
	atAnEarlierHead := baselineRun("9f8e7d6", "")
	atAnEarlierHead.ID = "r8"
	atAnEarlierHead.Passed = true

	for _, tc := range []struct {
		name   string
		stored []run.Record
	}{
		{
			name:   "a probe's own run at this head is passed over",
			stored: []run.Record{mutated},
		},
		{
			name:   "a first probe at a fresh head still obtains one",
			stored: []run.Record{atAnEarlierHead, mutated},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var performed []Spec
			resolved, err := Ensure(tc.stored, head, Gap, "",
				recordingPerformer(head, &performed))
			require.NoError(t, err)
			assert.Equal(t, []Spec{{}}, performed,
				"§5.2.6 performs the baseline no admissible run stands for")
			assert.Equal(t, "r1", resolved.ID(),
				"the baseline is the run just performed, not the probe's own")
		})
	}

	t.Run("a performed run that is itself inadmissible resolves to nothing", func(t *testing.T) {
		_, err := Ensure(nil, head, Gap, "", func(Spec) (run.Record, error) {
			return baselineRun(head, "", probed), nil
		})
		require.Error(t, err,
			"a probe graded against a baseline that does not stand is the "+
				"pre-existing failure §5.2.2 exists to keep off a pull request")
		assert.Contains(t, err.Error(), head)
	})
}
