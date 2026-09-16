package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/run"
	"github.com/deligoez/cr/internal/state"
)

// §5.2.2's definition: "a probe's baseline is a run on unmutated, un-probed
// code of the tests the probe runs — for a mutation probe, the run with the
// probe's filter and paths; for a gap probe, the run with the probe's paths and
// no filter, which is the whole suite when it has none". There is one baseline
// and not two: the whole suite is required only of a probe that runs the whole
// suite.
//
// A gap probe's filter is dropped from its baseline, and that is the round 8
// repair rather than an oversight — §5.5's `baseline` row says why, "since the
// probe's own test does not exist in the baseline". Its paths are not dropped,
// because §5.4.2 narrows the probe's own run to the file cr placed and leaves
// the `--path` values to name the population the baseline measures.
func TestAProbesBaselineIsTheTestsTheProbeRuns(t *testing.T) {
	const filter = "handles an empty cart"
	paths := []string{"tests/Feature", "tests/Unit/CartTest.php"}

	for _, tc := range []struct {
		name       string
		kind       Kind
		filter     string
		paths      []string
		referenced Spec
	}{
		{
			name:       "a filtered gap probe drops the filter",
			kind:       Gap,
			filter:     filter,
			referenced: Spec{},
		},
		{
			name:       "a filtered gap probe keeps its paths",
			kind:       Gap,
			filter:     filter,
			paths:      paths,
			referenced: Spec{Paths: paths},
		},
		{
			name:       "a filtered mutation probe keeps the filter",
			kind:       Mutation,
			filter:     filter,
			referenced: Spec{Filter: filter},
		},
		{
			name:       "a filtered and pathed mutation probe keeps both",
			kind:       Mutation,
			filter:     filter,
			paths:      paths,
			referenced: Spec{Filter: filter, Paths: paths},
		},
		{
			name:       "an unfiltered mutation probe measures the whole suite",
			kind:       Mutation,
			referenced: Spec{},
		},
		{
			name:       "an unfiltered gap probe measures the same one run",
			kind:       Gap,
			referenced: Spec{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.referenced, Referenced(tc.kind, tc.filter, tc.paths),
				"§5.5: the run the probe record's baseline field names")
			assert.Equal(t, []Spec{tc.referenced}, Required(tc.kind, tc.filter, tc.paths),
				"§5.2.2: a probe has one baseline, and it measures the tests the probe runs")
		})
	}
}

// §5.2.2's "once per head, sandbox generation, filter, and paths", which is a
// question asked of runs.ndjson rather than of a memo beside it: a baseline is
// recorded when a run record at this head, carrying this filter and these paths
// and no probe, is already there. The generation is asked first, by OfSandbox,
// and is not a condition here.
//
// The head clause is what makes a moved head invalidate every baseline for
// free, and §5.5.3 is why it has to — records from earlier heads stay on disk.
// The probe clause is §5.2.6's, and the contamination clause is round 12's
// baseline-contamination finding: a run whose sandbox failed §5.1.6's check
// afterwards measured something other than the head, so it is no more a
// baseline for it than a run at a different head is.
func TestABaselineIsRecordedOncePerHeadFilterAndPaths(t *testing.T) {
	const head = "0a1b2c3"
	const filter = "handles an empty cart"
	paths := []string{"tests/Feature"}
	required := Required(Mutation, filter, paths)
	require.Equal(t, []Spec{{Filter: filter, Paths: paths}}, required,
		"the fixture is a filtered and pathed mutation probe's one baseline")

	for _, tc := range []struct {
		name    string
		stored  []run.Record
		missing []Spec
	}{
		{
			name:    "nothing recorded yet leaves it to perform",
			stored:  nil,
			missing: required,
		},
		{
			name:    "the same filter and paths at this head leave nothing to perform",
			stored:  []run.Record{baselineRun(head, filter, paths)},
			missing: []Spec{},
		},
		{
			name:    "a run at an earlier head is no baseline for this one",
			stored:  []run.Record{baselineRun("9f8e7d6", filter, paths)},
			missing: required,
		},
		{
			name:    "§5.2.6: a run carrying a probe cannot stand as one",
			stored:  []run.Record{baselineRun(head, filter, paths, probed)},
			missing: required,
		},
		{
			name:    "a contaminated run cannot stand as one either",
			stored:  []run.Record{baselineRun(head, filter, paths, contaminated)},
			missing: required,
		},
		{
			name:    "the same filter over the whole suite measured another population",
			stored:  []run.Record{baselineRun(head, filter, nil)},
			missing: required,
		},
		{
			name:    "the same paths under another filter measured another population",
			stored:  []run.Record{baselineRun(head, "", paths)},
			missing: required,
		},
		{
			name:    "the same paths in another order measured another population",
			stored:  []run.Record{baselineRun(head, filter, []string{"tests/Unit", "tests/Feature"})},
			missing: required,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.missing, Missing(tc.stored, head, required))
		})
	}
}

// §5.2.6's last sentence: "A run record carrying no `sandbox` matches no
// sandbox generation." Both directions are refused — a record naming no
// generation is not a run of any sandbox cr can identify, and an empty
// generation names no sandbox to look for runs of.
func TestARunCarryingNoSandboxMatchesNoGeneration(t *testing.T) {
	const generation = "2026-09-16T09:00:00Z"
	ungenerated := run.Record{ID: "r1", Stamp: state.Stamp{Head: "0a1b2c3", Round: 1}}
	measured := ungenerated
	measured.ID, measured.Sandbox = "r2", generation

	stored := []run.Record{ungenerated, measured}
	assert.Equal(t, []run.Record{measured}, OfSandbox(stored, generation),
		"a record naming no generation is not a run of this sandbox")
	assert.Equal(t, []run.Record{}, OfSandbox(stored, ""),
		"and no generation matches no record, rather than every record that names none")
}

// baselineRun is a run record that stands as a baseline: at head, carrying
// filter and paths, on un-probed and uncontaminated code.
func baselineRun(head, filter string, paths []string, mark ...func(*run.Record)) run.Record {
	record := run.Record{
		ID:     "r1",
		Stamp:  state.Stamp{Head: head, Round: 1},
		Filter: filter,
		Paths:  paths,
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
