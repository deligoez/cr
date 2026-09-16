package intent

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// specClaimFields is §3.3's table, transcribed from the spec in its own order,
// with each row's answer in the Required column. head is the column's "yes"
// that §2.3.3 takes out of the agent's hands; round is not a row of this table
// at all, and TestAClaimCarriesExactlyTheTableRows is where it is accounted
// for.
var specClaimFields = []ClaimField{
	{Name: "id", Requirement: Required},
	{Name: "text", Requirement: Required},
	{Name: "source", Requirement: Required},
	{Name: "span", Requirement: Required},
	{Name: "note_id", Requirement: Optional},
	{Name: "file", Requirement: Optional},
	{Name: "span_hash", Requirement: Computed},
	{Name: "issue_hash", Requirement: Computed},
	{Name: "head", Requirement: Stamped},
}

// §3.3's table is what the required-field walk and the computed fence are read
// against, so the table cr carries has to be §3.3's own — every row, in order,
// with the answer the Required column gives it. A row that quietly changed its
// answer would change what an agent is allowed to write without anything else
// in cr noticing, and the two hashes are the rows that show why: §3.3.3 detects
// drift by comparing a stored `issue_hash` against a fresh one, so an agent
// that could write that row could make a changed issue report no drift at all.
func TestTheClaimFieldTableIsTheOneTheSpecWrites(t *testing.T) {
	assert.Equal(t, specClaimFields, ClaimFields())
}

// The table decides nothing on its own: a validator that clears every row of it
// still lets a claim through carrying a field the table never named, and drops
// one the table promised. Walking the struct reflectively catches either,
// including a field added years from now.
//
// `round` is the one wire key §3.3's table does not list, and it is expected
// here rather than added to the table. §2.3.3 requires it on every record of
// claims.ndjson and state.Stamp is what carries it, so a claim that did not
// hold it could not be written at all — while a table that listed it would
// claim §3.3 wrote a row §3.3 never wrote.
func TestAClaimCarriesExactlyTheTableRows(t *testing.T) {
	names := make([]string, 0, len(specClaimFields)+1)
	for _, field := range specClaimFields {
		names = append(names, field.Name)
	}
	assert.ElementsMatch(t, append(names, "round"), wireNames(reflect.TypeFor[Claim]()))
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
