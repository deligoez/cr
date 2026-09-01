package coverage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// A cell may not supply the fields cr computes onto it.
//
// §4.5.5 has a cell carry "the head it was filled against" and "the unit hash
// of §3.4.6 it was filled for", and §10.2.2 then asks whether each cell "was
// filled for that unit's current unit hash". Both of those are the guard's own
// input, so an agent free to write them is an agent free to answer the question
// being asked of it: a cell echoing the current hash onto a verdict reached
// against older code is indistinguishable from one filled a moment ago, and
// P6's "coverage is proven" reduces to coverage asserted.
//
// §6.1.4 is the treatment, and it is a rejection rather than a silent
// overwrite: cr names the line and the field so the author of the file learns
// that the value they recorded was never theirs to record.
//
// The value is not looked at. A correct hash, a wrong one, an empty one and a
// null are the same fault, because what is wrong is the authorship and not the
// number — a check that accepted the right answer would be reading the agent's
// reply to a question the agent may not be asked.
func TestACellMayNotSupplyTheFieldsCrComputes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		line  string
		field string
	}{
		{
			name:  "the current unit hash",
			line:  `{"unit":"u1","role":"correctness","result":"pass","unit_hash":"38372bc96eb4010e"}`,
			field: "unit_hash",
		},
		{
			name:  "a unit hash that is no unit's",
			line:  `{"unit":"u1","role":"correctness","result":"pass","unit_hash":"0000000000000000"}`,
			field: "unit_hash",
		},
		{
			name:  "an empty unit hash",
			line:  `{"unit":"u1","role":"correctness","result":"pass","unit_hash":""}`,
			field: "unit_hash",
		},
		{
			name:  "a null unit hash",
			line:  `{"unit":"u1","role":"correctness","result":"pass","unit_hash":null}`,
			field: "unit_hash",
		},
		{
			name:  "the head it was filled against",
			line:  `{"unit":"u1","role":"correctness","result":"pass","head":"be7e2c75aeb661ba"}`,
			field: "head",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cells, err := Decode(cellsFile, []byte(tc.line+"\n"), units(), active())

			var reserved *state.ReservedFieldError
			require.ErrorAs(t, err, &reserved)
			assert.Empty(t, cells, "a refused file stores nothing at all")
			assert.Equal(t, tc.field, reserved.Field)
			assert.Equal(t, cellsFile, reserved.File)
			assert.Equal(t, 1, reserved.Line)
			assert.Contains(t, err.Error(), "written by cr and must not be supplied")
		})
	}
}

