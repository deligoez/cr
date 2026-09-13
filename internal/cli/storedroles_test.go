package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/state"
)

// §4.5.1's stored active set, read back through the two commands that consume
// it: `cr cells record` refuses a cell of a role outside it and accepts one of
// each role inside it, and `cr status` counts a row complete against exactly
// those roles.
//
// brief's TestTheActiveRoleSetIsStoredWithoutAnyFanOut asserts meta.json holds
// the definition; this asserts the readers agree with each other. The stored
// set is deliberately not the one the definition yields for the round —
// `generic` leaves convention active, and the set here drops it — so a command
// that derived its own set instead of reading meta.json's would disagree: a
// cells record accepting the convention cell, or a status demanding it and
// counting u2 a gap against three roles.
func TestCellsRecordAndStatusAgreeOnTheStoredActiveRoles(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.ActiveRoles = []string{"correctness", "intent-coverage"}
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())

	cells := func(lines ...string) error {
		t.Helper()
		path := filepath.Join(t.TempDir(), "cells.ndjson")
		require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
		return runCLI(t, "cells", "record", fixturePR, path, "--repo", fixtureSlug)
	}

	refused := cells(`{"unit":"u2","role":"convention","result":"pass"}`)
	var rejected *coverage.RejectedCellError
	require.ErrorAs(t, refused, &rejected, "§4.5.6 rejects a cell of a role outside the stored set")
	assert.Equal(t, "role", rejected.Field)
	assert.Equal(t, ExitValidation, exitCodeFor(refused))

	require.NoError(t, cells(
		`{"unit":"u2","role":"correctness","result":"pass"}`,
		`{"unit":"u2","role":"intent-coverage","result":"pass"}`,
	), "every role of the stored set fills a cell")

	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report struct {
		Coverage coverage.Rows `json:"coverage"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	assert.Equal(t, coverage.Rows{Units: 2, Complete: 2, Gaps: 0, Oversized: 1, Roles: 2}, report.Coverage,
		"§10.2.2 counts u2 complete against the two roles cells record accepted, and demands no convention cell")
}
