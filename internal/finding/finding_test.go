package finding

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// specFields is §6.1's table, transcribed from the spec in its own order, with
// each row's answer in the Required column. head and round are the column's
// "yes" that §2.3.3 takes out of the agent's hands.
var specFields = []Field{
	{Name: "id", Requirement: Required},
	{Name: "kind", Requirement: Required},
	{Name: "axis", Requirement: Computed},
	{Name: "role", Requirement: Required},
	{Name: "class", Requirement: Required},
	{Name: "rule", Requirement: Optional},
	{Name: "severity", Requirement: Required},
	{Name: "grade", Requirement: Computed},
	{Name: "unit", Requirement: Required},
	{Name: "claim", Requirement: Optional},
	{Name: "anchor", Requirement: Required},
	{Name: "summary", Requirement: Required},
	{Name: "evidence", Requirement: Required},
	{Name: "citations", Requirement: Optional},
	{Name: "probe", Requirement: Optional},
	{Name: "suggestion", Requirement: Optional},
	{Name: "suggestion_origin", Requirement: Optional},
	{Name: "state", Requirement: Computed},
	{Name: "disposition", Requirement: Optional},
	{Name: "duplicate_of", Requirement: Optional},
	{Name: "suppressed_by", Requirement: Optional},
	{Name: "thread_id", Requirement: Optional},
	{Name: "round", Requirement: Stamped},
	{Name: "head", Requirement: Stamped},
}

// §6.1's table is what §6.1.3 and §6.1.4 are read against, so the table cr
// carries has to be §6.1's own — every row, in order, with the answer the
// Required column gives it. A row that quietly changed its answer would change
// what an agent is allowed to write without anything else in cr noticing, and
// axis is the row that shows why: it is computed, and §6.2's cited grade turns
// on the axis not being test.
func TestTheFieldTableIsTheOneTheSpecWrites(t *testing.T) {
	assert.Equal(t, specFields, Fields())
	assert.Equal(t, []Field{
		{Name: "path", Requirement: Required},
		{Name: "line", Requirement: Required},
		{Name: "content_hash", Requirement: Computed},
		{Name: "origin", Requirement: Computed},
	}, CitationFields())
}

// The table decides nothing on its own: a validator that clears every row of it
// still lets a record through carrying a field the table never named, and drops
// one the table promised. Walking the struct reflectively catches either,
// including a field added years from now.
func TestAFindingCarriesExactlyTheTableRows(t *testing.T) {
	names := make([]string, 0, len(specFields))
	for _, field := range specFields {
		names = append(names, field.Name)
	}
	assert.ElementsMatch(t, names, wireNames(reflect.TypeFor[Finding]()))
}

// §6.1 resolves every citation against the current head, so an entry carries no
// side. A side would be a second answer to a question already settled, and the
// wrong one would move a location across §6.2.1's unit boundary — which is the
// whole difference between a cited grade and an argued one.
func TestACitationCarriesNoSide(t *testing.T) {
	names := make([]string, 0, len(citationFields))
	for _, field := range CitationFields() {
		names = append(names, field.Name)
	}
	assert.ElementsMatch(t, names, wireNames(reflect.TypeFor[Citation]()))
	assert.NotContains(t, names, "side")
}

// wireNames returns the JSON key of every field of a record type, flattening an
// embedded struct because its fields are rows of the same table. A named struct
// or a slice is one row, whatever it holds.
func wireNames(t reflect.Type) []string {
	names := make([]string, 0, t.NumField())
	for _, field := range reflect.VisibleFields(t) {
		if field.Anonymous {
			continue
		}
		names = append(names, strings.Split(field.Tag.Get("json"), ",")[0])
	}
	return names
}

// §12.3 lets a slice serialise as [] and never as null, and a record holds
// three: its citations and its anchor's two context windows. Nil is the normal
// state of all three — most records cite nothing — so the encoding has to hold
// for a record that filled none of them, which is exactly the case a filled
// fixture would not test.
func TestARecordNeverSerialisesASliceAsNull(t *testing.T) {
	line, err := json.Marshal(Finding{})
	require.NoError(t, err)
	assert.NotContains(t, string(line), "null")

	filled, err := json.Marshal(Finding{
		Citations: []Citation{{Path: "app/Models/User.php", Line: 12}},
		Anchor:    Anchor{ContextBefore: []string{"before"}, ContextAfter: []string{"after"}},
	})
	require.NoError(t, err)
	assert.Contains(t, string(filled), `"citations":[{"path":"app/Models/User.php","line":12}]`)
}

// The struct is the resolved record, not the line an agent hands in, and head
// and round are where the two forms already come apart: §2.3.3 has cr write the
// pair and state.DecodeStamped refuses a line that supplied either, while
// state's stamped writers are what put it on the stored record. A record type
// inherits both only by embedding state.Stamp, and §6.1.4's wider rejection is
// built on the same separation, so it has to hold for a findings.ndjson line.
func TestHeadAndRoundReachARecordOnlyFromTheWriter(t *testing.T) {
	_, err := state.DecodeStamped[Finding](
		state.FileFindings, []byte(`{"id":"f1","class":"missing-test","head":"0f1e2d3"}`), nil,
	)
	var reserved *state.ReservedFieldError
	require.ErrorAs(t, err, &reserved)
	assert.Equal(t, "head", reserved.Field)

	records, err := state.DecodeStamped[Finding](
		state.FileFindings, []byte(`{"id":"f1","class":"missing-test"}`), nil,
	)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, state.Stamp{}, records[0].Stamp, "the wire form carries no stamp")

	l := state.New(t.TempDir())
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	at := state.Stamp{Head: "0f1e2d3", Round: 2}
	require.NoError(t, state.ReplaceStamped(held, state.FileFindings, at, records))
	require.NoError(t, held.Unlock())

	stored, err := state.ReadRecords[Finding](l, "acme", "web", 42, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, at, stored[0].Stamp)
	assert.Equal(t, "f1", stored[0].ID)
}
