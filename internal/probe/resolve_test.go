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
