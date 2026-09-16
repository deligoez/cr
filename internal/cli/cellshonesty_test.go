package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// `cr cells record` owes §2.4.6's and §2.5.2's reports and made neither.
//
// It is the command the two clauses were checked against one load site at a
// time rather than one command at a time, and it is the one that had been
// missed: §4.5.5 routes each cell's `coverage` object onto an axis through
// §2.5.5's corpus, and §4.6.5's hold on the intent pass is decided from the
// round's profile — two files loaded, and no honesty channel to say so. Both
// clauses bind every command that loads one of those files, not the ones whose
// output happens to be about it.
func TestCellsRecordReportsAStaleProfileAndAStaleEjectedRole(t *testing.T) {
	previousRole, _ := staleRoleBytes(t)
	previousProfile, err := os.ReadFile(
		filepath.Join("..", "profile", "builtin", "shipped", "v0.2.1", "laravel-pest.json"))
	require.NoError(t, err)
	require.NotEqual(t, profile.Builtins()["laravel-pest"], string(previousProfile),
		"the fixture is only a test while v0.2.1's laravel-pest differs from this build's")

	layout := briefedForCells(t)
	// The round resolved laravel-pest, so the profile file the command
	// loads is the one written stale below.
	held, err := layout.LockPR(cellsOwner, cellsRepo, cellsPR)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: cellsOwner, Repo: cellsRepo, PR: cellsPR,
		IssueKey: "CR-7", Round: 1, Head: cellsHead, ProfileID: "laravel-pest",
		ActiveRoles:  []string{"convention", "correctness"},
		MappingRound: 1, MappingHead: cellsHead,
	}))
	require.NoError(t, held.Unlock())
	require.NoError(t, layout.EnsureProfile("laravel-pest", string(previousProfile)))
	require.NoError(t, os.MkdirAll(layout.RolesDir(), 0o700))
	require.NoError(t, os.WriteFile(layout.Role("correctness"), previousRole, 0o600))

	path := filepath.Join(t.TempDir(), "cells.ndjson")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"unit":"u1","role":"correctness","result":"pass"}`+"\n"), 0o600))
	printed := throughAPipe(t, "cells", "record", strconv.Itoa(cellsPR), path, "--repo", cellsSlug)

	var payload struct {
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &payload))
	require.Len(t, payload.Honesty, 2, "one sentence for the profile and one for the role")
	assert.Contains(t, payload.Honesty[0],
		layout.Profile("laravel-pest")+" is the laravel-pest profile cr v0.2.1 shipped, unedited",
		"§2.4.6: the release and the file are named")
	assert.Contains(t, payload.Honesty[0], "cr init updates the file to it",
		"§2.4.6: and so is cr init")
	assert.Contains(t, payload.Honesty[1],
		layout.Role("correctness")+" is the correctness role cr v0.2.2 shipped, unedited",
		"§2.5.2: the release and the file are named")
	assert.Contains(t, payload.Honesty[1], "cr init updates the file to it",
		"§2.5.2: and so is cr init")
}
