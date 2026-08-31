package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/run"
	"github.com/deligoez/cr/internal/state"
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

// §5.2.2's "once per head", which is a question asked of runs.ndjson rather
// than of a memo beside it: a baseline is recorded when a run record at this
// head, carrying this filter and no probe, is already there.
//
// The head clause is what makes a moved head invalidate every baseline for
// free, and §5.5.3 is why it has to — records from earlier heads stay on disk.
// The probe clause is §5.2.6's, and the contamination clause is round 12's
// baseline-contamination finding: a run whose sandbox failed §5.1.6's check
// afterwards measured something other than the head, so it is no more a
// baseline for it than a run at a different head is.
func TestABaselineIsRecordedOncePerHead(t *testing.T) {
	const head = "0a1b2c3"
	filtered := Required(Mutation, "handles an empty cart")
	require.Len(t, filtered, 2, "the fixture is a filtered mutation probe's pair")

	for _, tc := range []struct {
		name    string
		stored  []run.Record
		missing []Spec
	}{
		{
			name:    "nothing recorded yet leaves both to perform",
			stored:  nil,
			missing: filtered,
		},
		{
			name: "both recorded at this head leave nothing to perform",
			stored: []run.Record{
				baselineRun(head, ""),
				baselineRun(head, "handles an empty cart"),
			},
			missing: []Spec{},
		},
		{
			name:    "a run at an earlier head is no baseline for this one",
			stored:  []run.Record{baselineRun("9f8e7d6", "")},
			missing: filtered,
		},
		{
			name: "§5.2.6: a run carrying a probe cannot stand as one",
			stored: []run.Record{
				baselineRun(head, "", probed),
				baselineRun(head, "handles an empty cart", probed),
			},
			missing: filtered,
		},
		{
			name:    "a contaminated run cannot stand as one either",
			stored:  []run.Record{baselineRun(head, "", contaminated)},
			missing: filtered,
		},
		{
			name:    "the unfiltered run alone leaves the filtered one to perform",
			stored:  []run.Record{baselineRun(head, "")},
			missing: []Spec{{Filter: "handles an empty cart"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.missing, Missing(tc.stored, head, filtered))
		})
	}
}

// baselineRun is a run record that stands as a baseline: at head, carrying
// filter, on un-probed and uncontaminated code.
func baselineRun(head, filter string, mark ...func(*run.Record)) run.Record {
	record := run.Record{
		ID:     "r1",
		Stamp:  state.Stamp{Head: head, Round: 1},
		Filter: filter,
	}
	for _, apply := range mark {
		apply(&record)
	}
	return record
}

// probed marks a run as having measured a probe's code, which §5.2.6 fences out.
func probed(record *run.Record) { record.Probe = "p1" }

// contaminated marks a run whose sandbox failed §5.1.6's check after it.
func contaminated(record *run.Record) { record.Contaminated = true }
