package intent

import (
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
