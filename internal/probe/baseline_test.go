package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// §5.2.2's two sentences, and the round 8 repair that scopes the second one.
// The unfiltered run is required of every probe; the filtered run joins it only
// for a filtered mutation probe. §5.5's `baseline` row names which of the two
// the probe record points at, and for a gap probe that is the unfiltered run,
// "since the probe's own test does not exist in the baseline".
func TestOnlyAFilteredMutationProbeAddsASecondBaseline(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kind       Kind
		filter     string
		required   []Spec
		referenced Spec
	}{
		{
			name:       "a filtered gap probe records no filtered baseline",
			kind:       Gap,
			filter:     "handles an empty cart",
			required:   []Spec{{}},
			referenced: Spec{},
		},
		{
			name:       "a filtered mutation probe records one",
			kind:       Mutation,
			filter:     "handles an empty cart",
			required:   []Spec{{}, {Filter: "handles an empty cart"}},
			referenced: Spec{Filter: "handles an empty cart"},
		},
		{
			name:       "an unfiltered mutation probe needs the unfiltered run once",
			kind:       Mutation,
			filter:     "",
			required:   []Spec{{}},
			referenced: Spec{},
		},
		{
			name:       "an unfiltered gap probe needs the same one run",
			kind:       Gap,
			filter:     "",
			required:   []Spec{{}},
			referenced: Spec{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.required, Required(tc.kind, tc.filter),
				"§5.2.2: the unfiltered run always, the filtered one additionally")
			assert.Equal(t, tc.referenced, Referenced(tc.kind, tc.filter),
				"§5.5: the run the probe record's baseline field names")
		})
	}
}
