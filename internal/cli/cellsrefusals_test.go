package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/state"
)

// cellsRound rewrites briefedForCells' meta.json with the active roles and the
// issue key given, and with the mapping stamp only when mapped is true, so each
// refusal below meets the round it is about.
func cellsRound(t *testing.T, issueKey string, mapped bool, roles ...string) state.Layout {
	t.Helper()
	layout := briefedForCells(t)
	meta := &state.Meta{
		Owner: cellsOwner, Repo: cellsRepo, PR: cellsPR,
		IssueKey: issueKey, Round: 1, Head: cellsHead, ActiveRoles: roles,
	}
	if mapped {
		meta.MappingRound, meta.MappingHead = 1, cellsHead
	}
	held, err := layout.LockPR(cellsOwner, cellsRepo, cellsPR)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(meta))
	require.NoError(t, held.Unlock())
	return layout
}

// recordCellsFile writes the lines to a file of their own and hands it to
// `cr cells record`, returning the path so a refusal can be compared whole.
func recordCellsFile(t *testing.T, lines ...string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cells.ndjson")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	return path, runCLI(t, "cells", "record", strconv.Itoa(cellsPR), path, "--repo", cellsSlug)
}

// storedCoverage is coverage.ndjson's bytes, and the empty string while no
// recording has created it.
func storedCoverage(t *testing.T, layout state.Layout) string {
	t.Helper()
	body, err := os.ReadFile(layout.PRFile(cellsOwner, cellsRepo, cellsPR, state.FileCoverage))
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	require.NoError(t, err)
	return string(body)
}

// A file naming one (unit, role) twice is refused with exit 1 naming both
// lines, and the round keeps what it held: §4.5.6 replaces the one cell a seat
// holds, and storing both verdicts left QA's u2/correctness a pass and a
// question at once (D-S04-2).
func TestCellsRecordRefusesAFileNamingOneSeatTwice(t *testing.T) {
	layout := briefedForCells(t)
	require.NoError(t, recordCells(t, `{"unit":"u1","role":"correctness","result":"pass"}`))
	before := storedCoverage(t, layout)

	path, err := recordCellsFile(t,
		`{"unit":"u2","role":"correctness","result":"question"}`,
		`{"unit":"u2","role":"correctness","result":"pass"}`)

	var rejected *coverage.RejectedCellError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, path+` line 2: unit "u2" and role "correctness" repeat the seat of line 1; `+
		`§4.5.6 replaces the one cell a (unit, role) holds, so give each seat one cell in the file`,
		err.Error())
	assert.Equal(t, before, storedCoverage(t, layout), "a refused file stores nothing")
	assert.Equal(t, []string{"u1/correctness"}, filledCells(t, layout))
}

// A test-axis cell whose coverage object gives no test paths is refused with
// exit 1 rather than stored as resting on `[]` nobody wrote (D-S04-3), and so
// is an empty list beside a verdict that tests exercise the unit. An uncovered
// verdict can rest on none, and says so with an explicit `[]`.
func TestCellsRecordRefusesACoverageVerdictWithNoTestPaths(t *testing.T) {
	const fixed = `{"unit":"u1","role":"test-adequacy","result":"pass","coverage":`
	for _, tc := range []struct {
		name     string
		coverage string
		problem  string
	}{
		{"no test_paths key", `{"classification":"covered"}`,
			"is required by §4.5.5, and §4.4.1 records the classification together with the test " +
				"paths it rested on; write [] only when an uncovered verdict rested on none"},
		{"a null test_paths", `{"classification":"covered","test_paths":null}`,
			"is required by §4.5.5, and §4.4.1 records the classification together with the test " +
				"paths it rested on; write [] only when an uncovered verdict rested on none"},
		{"an empty list beside covered", `{"classification":"covered","test_paths":[]}`,
			"is empty, and the classification is covered; §4.4.1 records the classification " +
				"together with the test paths it rested on, so name the tests that exercise the unit"},
		{"an empty list beside partially-covered", `{"classification":"partially-covered","test_paths":[]}`,
			"is empty, and the classification is partially-covered; §4.4.1 records the classification " +
				"together with the test paths it rested on, so name the tests that exercise the unit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout := cellsRound(t, "CR-7", true, "correctness", "test-adequacy")

			_, err := recordCellsFile(t, fixed+tc.coverage+"}")

			var rejected *coverage.RejectedCellError
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Equal(t, 1, rejected.Line)
			assert.Equal(t, "coverage.test_paths", rejected.Field)
			assert.Equal(t, tc.problem, rejected.Problem)
			assert.Empty(t, storedCoverage(t, layout), "a refused file stores nothing")
		})
	}

	t.Run("an uncovered verdict resting on no test path", func(t *testing.T) {
		layout := cellsRound(t, "CR-7", true, "correctness", "test-adequacy")
		_, err := recordCellsFile(t, fixed+`{"classification":"uncovered","test_paths":[]}}`)
		require.NoError(t, err)
		assert.Equal(t, []string{"u1/test-adequacy"}, filledCells(t, layout))
	})
}

// A key the cell schema does not define is refused with exit 1, at the top of
// the line and inside its coverage object, where a key given twice is refused
// too: encoding/json would drop the one and keep parts of both copies of the
// other, and the stored cell would not be the line its author wrote.
func TestCellsRecordRefusesAKeyTheCellSchemaDoesNotDefine(t *testing.T) {
	t.Run("at the top of the line", func(t *testing.T) {
		layout := briefedForCells(t)
		path, err := recordCellsFile(t, `{"unit":"u1","role":"correctness","result":"pass","comment":"x"}`)

		var rejected *coverage.RejectedCellError
		require.ErrorAs(t, err, &rejected)
		assert.Equal(t, ExitValidation, exitCodeFor(err))
		assert.Equal(t, path+" line 1: comment is not a cell field; §4.5.5 has the agent write exactly "+
			"unit, role, result, reason, note_id, coverage, and cr writes unit_hash, head and round itself",
			err.Error())
		assert.Empty(t, storedCoverage(t, layout))
	})

	t.Run("inside the coverage object", func(t *testing.T) {
		layout := cellsRound(t, "CR-7", true, "correctness", "test-adequacy")
		_, err := recordCellsFile(t, `{"unit":"u1","role":"test-adequacy","result":"pass",`+
			`"coverage":{"classification":"covered","test_paths":["t.php"],"note":"x"}}`)

		var rejected *coverage.RejectedCellError
		require.ErrorAs(t, err, &rejected)
		assert.Equal(t, ExitValidation, exitCodeFor(err))
		assert.Equal(t, "coverage.note", rejected.Field)
		assert.Equal(t, "is not a field of §4.5.5's coverage object, which has exactly "+
			"classification, test_paths", rejected.Problem)
		assert.Empty(t, storedCoverage(t, layout))
	})

	t.Run("a coverage key given twice", func(t *testing.T) {
		layout := cellsRound(t, "CR-7", true, "correctness", "test-adequacy")
		_, err := recordCellsFile(t, `{"unit":"u1","role":"test-adequacy","result":"pass",`+
			`"coverage":{"classification":"uncovered","test_paths":["t.php"],"classification":"covered"}}`)

		var repeated *state.RepeatedKeyError
		require.ErrorAs(t, err, &repeated)
		assert.Equal(t, ExitValidation, exitCodeFor(err))
		assert.Equal(t, 1, repeated.Line)
		assert.Equal(t, "coverage.classification", repeated.Key)
		assert.Empty(t, storedCoverage(t, layout))
	})
}

// An na cell at a seat where the round holds a record from that role on that
// unit is refused with exit 1, as a pass is: it says the role had nothing to
// say about the unit beside a record it raised there. An na at a seat no record
// was raised at stands.
func TestCellsRecordRefusesAnNaCellWhereItsRoleRaisedARecord(t *testing.T) {
	layout := recordedHomeWithCells(t)
	coverageFile := layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileCoverage)
	before, err := os.ReadFile(coverageFile)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "cells.ndjson")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"unit":"u2","role":"correctness","result":"na","reason":"generated code"}`+"\n"), 0o600))

	err = runCLI(t, "cells", "record", recordPR, path, "--repo", recordSlug)

	var rejected *coverage.RejectedCellError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, 1, rejected.Line)
	assert.Equal(t, "result", rejected.Field)
	assert.Equal(t, `is na at unit "u2" and role "correctness", and this round holds record(s) f1 `+
		`from that role on that unit; a role that raised a record there had something to say about `+
		`it, so file the cell as finding or question`, rejected.Problem)
	after, err := os.ReadFile(coverageFile)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "a refused file stores nothing")

	recordPassCells(t, `{"unit":"u1","role":"correctness","result":"na","reason":"generated code"}`)
	stored, err := state.ReadRecords[coverage.Cell](layout, recordOwner, recordRepo, recordPRNum, state.FileCoverage)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, "u1", stored[0].Unit)
}

// Cells for any role off the intent axis are refused with exit 4 until the
// round's mapping is recorded, as `cr review` refuses to emit their prompts
// (§4.6.5); the intent pass's own cells are accepted, and a round whose intent
// axis is unavailable has no mapping to wait for (§4.6.6).
func TestCellsRecordRefusesTheRemainingAxesBeforeTheMapping(t *testing.T) {
	intentCell := `{"unit":"u1","role":"intent-coverage","result":"pass"}`
	correctnessCell := `{"unit":"u1","role":"correctness","result":"pass"}`

	t.Run("a correctness cell before the mapping", func(t *testing.T) {
		layout := cellsRound(t, "CR-7", false, "correctness", "intent-coverage")
		path, err := recordCellsFile(t, intentCell, correctnessCell)

		var required *coverage.MappingRequiredError
		require.ErrorAs(t, err, &required)
		assert.Equal(t, ExitState, exitCodeFor(err), "§11.2 codes where the round stands 4")
		assert.Equal(t, path+` line 2: role "correctness" is on the correctness axis, and round 1 at head `+
			cellsHead+" has no mapping; §4.6.5 refuses the remaining axes until the intent pass has "+
			"recorded one, so record the mapping with `cr map record` and record this cell again",
			err.Error())
		assert.Equal(t, "run the intent pass and record its mapping with `cr map record`, then record "+
			"the cells again", hintFor(err))
		assert.Empty(t, storedCoverage(t, layout), "the intent cell above it is not stored either")
	})

	for _, tc := range []struct{ role, axis, line string }{
		{"convention", "convention", `{"unit":"u1","role":"convention","result":"pass"}`},
		{"test-adequacy", "test", `{"unit":"u1","role":"test-adequacy","result":"pass",` +
			`"coverage":{"classification":"covered","test_paths":["t.php"]}}`},
	} {
		t.Run("a "+tc.role+" cell before the mapping", func(t *testing.T) {
			layout := cellsRound(t, "CR-7", false, "convention", "intent-coverage", "test-adequacy")
			_, err := recordCellsFile(t, tc.line)

			var required *coverage.MappingRequiredError
			require.ErrorAs(t, err, &required)
			assert.Equal(t, ExitState, exitCodeFor(err))
			assert.Equal(t, tc.role, required.Role)
			assert.Equal(t, tc.axis, required.Axis)
			assert.Empty(t, storedCoverage(t, layout))
		})
	}

	t.Run("the intent pass's own cell before the mapping", func(t *testing.T) {
		layout := cellsRound(t, "CR-7", false, "correctness", "intent-coverage")
		_, err := recordCellsFile(t, intentCell)
		require.NoError(t, err)
		assert.Equal(t, []string{"u1/intent-coverage"}, filledCells(t, layout))
	})

	t.Run("a correctness cell once the mapping is recorded", func(t *testing.T) {
		layout := cellsRound(t, "CR-7", true, "correctness", "intent-coverage")
		_, err := recordCellsFile(t, correctnessCell)
		require.NoError(t, err)
		assert.Equal(t, []string{"u1/correctness"}, filledCells(t, layout))
	})

	t.Run("a correctness cell in a round with no issue key", func(t *testing.T) {
		layout := cellsRound(t, "", false, "correctness")
		_, err := recordCellsFile(t, correctnessCell)
		require.NoError(t, err)
		assert.Equal(t, []string{"u1/correctness"}, filledCells(t, layout))
	})
}
