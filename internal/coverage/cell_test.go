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

// An `na` cell that gives no reason is rejected, and so is a reason on a cell
// that is not `na`.
//
// §4.5.5 requires "a reason when it is `na`", and the requirement is load-
// bearing rather than tidy. `na` is the one verdict that says a lens had
// nothing to say about a unit, so it is the one that can shrink what P6 counts
// as proven coverage — a row of bare `na`s reads to §10.2.2 exactly like a row
// of filled cells while proving that nothing was looked at. The reason is what
// makes the shrinkage inspectable, so a cell that omits it is refused rather
// than stored and reported later.
func TestAnNaCellWithoutAReasonIsRejected(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
	}{
		{"no reason at all", `{"unit":"u1","role":"correctness","result":"na"}`},
		{"a null reason", `{"unit":"u1","role":"correctness","result":"na","reason":null}`},
		{"an empty reason", `{"unit":"u1","role":"correctness","result":"na","reason":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cells, err := Decode(cellsFile, []byte(tc.line+"\n"), active())

			var rejected *RejectedCellError
			require.ErrorAs(t, err, &rejected)
			assert.Empty(t, cells, "a refused file stores nothing at all")
			assert.Equal(t, cellsFile, rejected.File)
			assert.Equal(t, 1, rejected.Line)
			assert.Equal(t, "reason", rejected.Field)
			assert.Equal(t,
				"cells.ndjson line 1: reason is required by §4.5.5 when result is na, "+
					"because an unexplained na is a lens that did not look",
				err.Error())
		})
	}

	t.Run("a reason on a verdict that is not na", func(t *testing.T) {
		_, err := Decode(cellsFile,
			[]byte(`{"unit":"u1","role":"correctness","result":"pass","reason":"looked fine"}`+"\n"),
			active())

		var rejected *RejectedCellError
		require.ErrorAs(t, err, &rejected)
		assert.Equal(t, "reason", rejected.Field)
		assert.Contains(t, rejected.Problem, "attaches to na alone")
	})
}

// §4.5.5's `coverage` object belongs to a cell a test-axis role filled, and to
// no other.
//
// Both directions matter and they fail differently. A test-axis cell with no
// classification leaves §4.4.1's entire answer unrecorded — the role was asked
// to call the unit covered, partially covered, or uncovered, and the round then
// holds no record that it did. A cell on another axis carrying one is worse for
// the trust economy: it puts a coverage verdict in coverage.ndjson that no role
// on that axis was asked to reach and no probe of §5 backs, and §4.4.2 makes an
// unbacked test-adequacy assertion a question rather than a claim.
//
// The classification set is closed with them, because §10 reads the word: a
// fourth value would be a cell no report can count and no reader can act on.
func TestTheCoverageObjectBelongsToTheTestAxisAlone(t *testing.T) {
	for _, tc := range []struct {
		name  string
		line  string
		field string
		says  string
	}{
		{
			name:  "a test-axis cell with no classification",
			line:  `{"unit":"u1","role":"test-adequacy","result":"pass"}`,
			field: "coverage",
			says:  "is required by §4.5.5 when the role is on the test axis",
		},
		{
			name: "a cell off the test axis carrying one",
			line: `{"unit":"u1","role":"correctness","result":"pass",` +
				`"coverage":{"classification":"covered","test_paths":[]}}`,
			field: "coverage",
			says:  `is §4.4.1's answer for the test axis, and "correctness" is on the correctness axis`,
		},
		{
			name: "a classification outside §4.4.1's three",
			line: `{"unit":"u1","role":"test-adequacy","result":"pass",` +
				`"coverage":{"classification":"mostly","test_paths":[]}}`,
			field: "coverage.classification",
			says:  `is "mostly"; §4.4.1 closes it at covered, partially-covered, uncovered`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(cellsFile, []byte(tc.line+"\n"), active())

			var rejected *RejectedCellError
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, tc.field, rejected.Field)
			assert.Contains(t, rejected.Problem, tc.says)
		})
	}

	// An uncovered unit rests on no test path, and §12.3 has the empty list
	// serialise as [] rather than null.
	t.Run("an uncovered unit names no test path", func(t *testing.T) {
		cells, err := Decode(cellsFile,
			[]byte(`{"unit":"u1","role":"test-adequacy","result":"question",`+
				`"coverage":{"classification":"uncovered"}}`+"\n"), active())
		require.NoError(t, err)
		require.Len(t, cells, 1)

		require.NotNil(t, cells[0].Coverage)
		assert.Equal(t, Uncovered, cells[0].Coverage.Classification)
		assert.Equal(t, []string{}, cells[0].Coverage.TestPaths)
	})
}

// A cell names the unit and the role it sits at, and says one of §4.5.5's four
// things about them.
//
// §1.3 defines a cell as the intersection of one unit and one active role, so a
// line missing either is not a cell at all: it is a verdict about nothing, and
// §10.1.1 could neither count it towards a unit's row nor report it as a gap.
// The verdict is held to the closed set for the same reason §1.5's axis ids are
// closed — every report in §10 reads the word, and a fifth value would be
// counted by none of them.
func TestACellNamesItsUnitItsRoleAndOneOfTheFourVerdicts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		line  string
		field string
		says  string
	}{
		{
			name:  "no unit",
			line:  `{"role":"correctness","result":"pass"}`,
			field: "unit",
			says:  "which has every cell name the unit it sits at",
		},
		{
			name:  "an empty unit",
			line:  `{"unit":"","role":"correctness","result":"pass"}`,
			field: "unit",
			says:  "which has every cell name the unit it sits at",
		},
		{
			name:  "no role",
			line:  `{"unit":"u1","result":"pass"}`,
			field: "role",
			says:  "which has every cell name the role it sits at",
		},
		{
			name:  "no result",
			line:  `{"unit":"u1","role":"correctness"}`,
			field: "result",
			says:  "is required by §4.5.5, which closes it at pass, finding, question, na",
		},
		{
			name:  "a verdict outside the four",
			line:  `{"unit":"u1","role":"correctness","result":"skipped"}`,
			field: "result",
			says:  `is "skipped"; §4.5.5 closes it at pass, finding, question, na`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cells, err := Decode(cellsFile, []byte(tc.line+"\n"), active())

			var rejected *RejectedCellError
			require.ErrorAs(t, err, &rejected)
			assert.Empty(t, cells, "a refused file stores nothing at all")
			assert.Equal(t, tc.field, rejected.Field)
			assert.Contains(t, rejected.Problem, tc.says)
		})
	}
}
