package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// briefedWithARecord is briefedForCells's round holding one record the
// correctness role raised on u1, as `cr record` leaves it.
func briefedWithARecord(t *testing.T) state.Layout {
	t.Helper()
	layout := briefedForCells(t)
	raised := aStoredRecord("f1", finding.StateDraft)
	require.Equal(t, "correctness", raised.Role)
	require.Equal(t, "u1", raised.Unit)
	held, err := layout.LockPR(cellsOwner, cellsRepo, cellsPR)
	require.NoError(t, err)
	require.NoError(t, state.ReplaceStamped(held, state.FileFindings,
		state.Stamp{Head: cellsHead, Round: 1}, []*finding.Finding{raised}))
	require.NoError(t, held.Unlock())
	return layout
}

// Round 8's cell-record-consistency-unchecked: a pass cell at a (unit, role)
// where the round holds a record from that role on that unit is refused with
// exit code 1, naming the cell and the record, and nothing in the file is
// written.
func TestAPassCellBesideARecordFromItsRoleOnItsUnitIsRefused(t *testing.T) {
	layout := briefedWithARecord(t)

	err := recordCells(t,
		`{"unit":"u2","role":"correctness","result":"pass"}`,
		`{"unit":"u1","role":"correctness","result":"pass"}`)

	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§11.2 codes a refused cell 1")
	var rejected *coverage.RejectedCellError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, 2, rejected.Line)
	assert.Equal(t, "result", rejected.Field)
	for _, named := range []string{`"u1"`, `"correctness"`, "f1"} {
		assert.Contains(t, err.Error(), named, "the refusal names the cell and the record")
	}
	assert.Empty(t, filledCells(t, layout), "the good line above the refused one is not written")
}

// The seat is the pair, not either half: a pass from another role on the unit,
// and a pass from the role on another unit, both stand beside the record.
func TestAPassCellAtAnotherSeatThanTheRecordIsAccepted(t *testing.T) {
	layout := briefedWithARecord(t)

	require.NoError(t, recordCells(t,
		`{"unit":"u1","role":"convention","result":"pass"}`,
		`{"unit":"u2","role":"correctness","result":"pass"}`,
		`{"unit":"u1","role":"correctness","result":"finding"}`))

	assert.Equal(t, []string{"u1/convention", "u2/correctness", "u1/correctness"}, filledCells(t, layout))
}

// The opposite direction is left unenforced: a finding or question cell at a
// seat holding no record is what §6.4.4's waiver drop and §9.3.6's posted-index
// drop leave behind, so it is accepted.
func TestAFindingCellWithNoSurvivingRecordIsAccepted(t *testing.T) {
	layout := briefedForCells(t)

	require.NoError(t, recordCells(t,
		`{"unit":"u1","role":"correctness","result":"finding"}`,
		`{"unit":"u2","role":"convention","result":"question"}`))

	assert.Equal(t, []string{"u1/correctness", "u2/convention"}, filledCells(t, layout))
}
