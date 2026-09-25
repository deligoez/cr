package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/state"
)

// twinnedForCells is briefedForCells with u2 recorded as §4.6.8's twin of u1:
// the pairing `cr brief` writes onto units.ndjson when u2's hunks repeat u1's.
func twinnedForCells(t *testing.T) state.Layout {
	t.Helper()
	layout := briefedForCells(t)
	held, err := layout.LockPR(cellsOwner, cellsRepo, cellsPR)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileUnits,
		[]byte(`{"id":"u1","path":"src/Order.php","hash":"38372bc96eb4010e",`+
			`"head":"`+cellsHead+`","round":1}`+"\n"+
			`{"id":"u2","path":"src/Money.php","hash":"0a1b2c3d4e5f6071","twin_of":"u1",`+
			`"head":"`+cellsHead+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// cellsFileOf writes the cells an agent hands `cr cells record` outside the
// state tree and returns its path.
func cellsFileOf(t *testing.T, lines string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cells.ndjson")
	require.NoError(t, os.WriteFile(path, []byte(lines+"\n"), 0o600))
	return path
}

// §4.6.8: a cell recorded at a unit some twin repeats is recorded at the twin
// too — the same result, the twin's own unit hash, and a reason naming the
// earlier unit — and the command reports the copy apart from the file's cells.
//
// Measured on a real pull request on a private Laravel repository: two
// event-test files received the identical change, and every role read both.
func TestACellRecordedAtAUnitIsRecordedAtItsTwin(t *testing.T) {
	layout := twinnedForCells(t)

	printed, err := runCLIPrinting(t, "cells", "record", "7",
		cellsFileOf(t, `{"unit":"u1","role":"correctness","result":"pass"}`), "--repo", cellsSlug)
	require.NoError(t, err)

	var reported cellsRecordResult
	require.NoError(t, json.Unmarshal([]byte(printed), &reported))
	require.Len(t, reported.Recorded, 1)
	require.Len(t, reported.Twinned, 1, "§4.6.8's copy is reported apart from the file's cells")

	stored, err := state.ReadRecords[coverage.Cell](layout, cellsOwner, cellsRepo, cellsPR, state.FileCoverage)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	twin := stored[1]
	assert.Equal(t, "u2", twin.Unit)
	assert.Equal(t, "correctness", twin.Role)
	assert.Equal(t, coverage.ResultPass, twin.Result, "§4.6.8: the same result")
	assert.Equal(t, "0a1b2c3d4e5f6071", twin.UnitHash, "the twin's own hash, which §10.2.2 compares")
	assert.Contains(t, twin.Reason, "twin of u1", "§4.6.8: a reason naming the earlier unit")
	assert.Equal(t, 1, twin.Round)
}

