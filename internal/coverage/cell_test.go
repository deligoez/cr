package coverage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/role"
)

// cellsFile is the name a decode is told it is reading, which every rejection
// has to name back.
const cellsFile = "cells.ndjson"

// active is a round's active roles of §4.5.1: one off the test axis and one on
// it, because §4.5.5's `coverage` row is conditional on exactly that difference
// and a fixture holding one kind could not exercise it.
func active() []role.Role {
	return []role.Role{
		{ID: "correctness", Title: "Correctness", Axis: axis.Correctness},
		{ID: "test-adequacy", Title: "Test adequacy", Axis: axis.Test},
	}
}

// §4.5.5's fields all reach coverage.ndjson, and the two conditional ones reach
// it under their condition.
//
// The whole row is asserted rather than the fields one case is about, because
// what §4.5.5 fixes is a record shape: a decoder that dropped `note_id` while
// keeping the rest would pass every test written about the field it kept, and
// §4.1.5 would then suppress an unmapped-unit question citing a note nothing
// recorded.
func TestACellCarriesEveryFieldSection455Names(t *testing.T) {
	body := []byte(`{"unit":"u1","role":"correctness","result":"pass"}` + "\n" +
		`{"unit":"u2","role":"correctness","result":"question","note_id":"CR-1#n1"}` + "\n" +
		`{"unit":"u3","role":"correctness","result":"na","reason":"the unit is generated"}` + "\n" +
		`{"unit":"u1","role":"test-adequacy","result":"finding",` +
		`"coverage":{"classification":"partially-covered","test_paths":["tests/OrderTest.php"]}}` + "\n")

	cells, err := Decode(cellsFile, body, active())
	require.NoError(t, err)
	require.Len(t, cells, 4)

	// §4.5.5: the unit id and role id the cell sits at, and the verdict.
	assert.Equal(t, "u1", cells[0].Unit)
	assert.Equal(t, "correctness", cells[0].Role)
	assert.Equal(t, ResultPass, cells[0].Result)
	assert.Empty(t, cells[0].Reason)

	// §4.1.5: the note that explained an unmapped unit.
	assert.Equal(t, "CR-1#n1", cells[1].NoteID)

	// §4.5.5: a reason when the verdict is `na`.
	assert.Equal(t, ResultNA, cells[2].Result)
	assert.Equal(t, "the unit is generated", cells[2].Reason)

	// §4.4.1: the classification and the test paths it rested on.
	require.NotNil(t, cells[3].Coverage)
	assert.Equal(t, PartiallyCovered, cells[3].Coverage.Classification)
	assert.Equal(t, []string{"tests/OrderTest.php"}, cells[3].Coverage.TestPaths)

	// §2.3.3's pair has one author and it is never the agent, so it is
	// unset here and stamped by the writer.
	for _, cell := range cells {
		assert.Empty(t, cell.Head)
		assert.Zero(t, cell.Round)
	}
}
