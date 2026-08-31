package probe

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// specFields is §5.5's field table, transcribed from the spec in its own order,
// with §2.3.3's `round` beside the `head` it travels with.
//
// It is written out again here rather than read from record.go, because a table
// checked against itself checks nothing: checkRecordFields holds the struct to
// the table, and this holds the table to the section.
var specFields = []field{
	{Name: "id"},
	{Name: "kind"},
	{Name: "head", Stamped: true},
	{Name: "round", Stamped: true},
	{Name: "input"},
	{Name: "filter"},
	{Name: "result"},
	{Name: "tests_run"},
	{Name: "tests_failed"},
	{Name: "baseline"},
	{Name: "target"},
	{Name: "duration_ms"},
	{Name: "output_tail"},
}

// §5.5's table is what a reader of probes.ndjson is promised, so the table cr
// carries has to be §5.5's own — every row, in order, and `head` and `round`
// marked as the two §2.3.3 takes out of every writer's hands.
func TestTheProbeFieldTableIsTheOneTheSpecWrites(t *testing.T) {
	assert.Equal(t, specFields, fields)
	assert.True(t, checkRecordFields(),
		"the struct is held to the table at package initialisation")
}

// The table decides nothing on its own: what a reader of probes.ndjson sees is
// the encoded line, and a row present in the table and dropped on the wire —
// or a key on the wire the table never named — is the drift §5.5 is written
// against. So the assertion is made against a marshalled record rather than
// against the struct a second reflective walk would report.
//
// The three rows §5.5's Required column answers "no" are asserted from the
// other side too: a record that derived no counts and narrowed to no filter
// omits exactly those three and no others, which is what keeps §5.3.4's fifth
// rung able to tell a count of zero from no count at all.
func TestEveryRowOfTheTableReachesTheWire(t *testing.T) {
	four, none := 4, 0
	filled := &Record{
		ID:          "p1",
		Kind:        Mutation,
		Stamp:       state.Stamp{Head: "9f2c1ab", Round: 2},
		Input:       "--- a/app.go\n+++ b/app.go\n",
		Filter:      "Retry",
		Result:      resultNoTestFailed,
		TestsRun:    &four,
		TestsFailed: &none,
		Baseline:    "r1",
		Target:      "app.go:3",
		DurationMS:  12,
		OutputTail:  "Tests:  4 passed\n",
	}
	named := make([]string, 0, len(specFields))
	for _, row := range specFields {
		named = append(named, row.Name)
	}
	assert.ElementsMatch(t, named, wireKeys(t, filled),
		"§5.5: every row of the table, and nothing the table does not name")

	sparse := *filled
	sparse.Filter, sparse.TestsRun, sparse.TestsFailed = "", nil, nil
	assert.ElementsMatch(t,
		slices.DeleteFunc(named, func(name string) bool {
			return name == "filter" || name == "tests_run" || name == "tests_failed"
		}),
		wireKeys(t, &sparse),
		"§5.5: the three rows the Required column answers no are the three that may be absent")
}

// wireKeys is the keys of one record as probes.ndjson holds them.
func wireKeys(t *testing.T, record *Record) []string {
	t.Helper()
	body, err := json.Marshal(record)
	require.NoError(t, err)
	var written map[string]any
	require.NoError(t, json.Unmarshal(body, &written))
	return slices.Collect(maps.Keys(written))
}

// §5.5's id space, allocated over every record probes.ndjson holds.
//
// It is read from the whole file rather than the current round's records, and
// the two ids below say why: §5.5.2 has a finding reference a probe by this id
// and §5.5.3 keeps records from earlier heads on disk, so an id an earlier round
// spent still names the probe a stored finding points at. Reusing it would
// silently repoint that finding at a different experiment.
func TestTheNextProbeIDIsAllocatedAboveEveryOneOnFile(t *testing.T) {
	for name, tc := range map[string]struct {
		existing []Record
		want     string
	}{
		"an empty file starts at one": {
			existing: nil,
			want:     "p1",
		},
		"the highest is taken, not the last": {
			existing: []Record{{ID: "p1"}, {ID: "p7"}, {ID: "p3"}},
			want:     "p8",
		},
		"an id from an earlier round is still spent": {
			existing: []Record{{ID: "p4"}},
			want:     "p5",
		},
		// The first id is the boundary of what counts as one at all:
		// a reader that refused p1 would allocate p1 a second time.
		"the first id is an id": {
			existing: []Record{{ID: "p1"}},
			want:     "p2",
		},
		"an id cr did not write contributes nothing": {
			existing: []Record{{ID: "r2"}, {ID: "p"}, {ID: "p0"}, {ID: "p-1"}, {ID: "p01"}, {ID: ""}},
			want:     "p1",
		},
		"a run id is not a probe id": {
			existing: []Record{{ID: "r9"}},
			want:     "p1",
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, NextID(tc.existing))
		})
	}
}
