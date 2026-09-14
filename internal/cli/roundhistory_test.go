package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// storedLine is the part of a units.ndjson or mapping.ndjson line these tests
// compare: which round and head wrote it, and what it names.
type storedLine struct {
	Round int    `json:"round"`
	Head  string `json:"head"`
	ID    string `json:"id,omitempty"`
	Path  string `json:"path,omitempty"`
	Claim string `json:"claim,omitempty"`
	Unit  string `json:"unit,omitempty"`
}

// storedLines reads one per-PR file back line by line, in file order.
func storedLines(t *testing.T, layout state.Layout, name string) []storedLine {
	t.Helper()
	body, err := os.ReadFile(layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, name))
	require.NoError(t, err)
	out := make([]storedLine, 0)
	for line := range strings.SplitSeq(string(body), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var stored storedLine
		require.NoErrorf(t, json.Unmarshal([]byte(line), &stored), "%s: %s", name, line)
		out = append(out, stored)
	}
	return out
}

// roundStatus is the part of `cr status` that says what one round holds.
type roundStatus struct {
	Round    int `json:"round"`
	Coverage struct {
		Units int `json:"units"`
	} `json:"coverage"`
	Intent struct {
		Claims int `json:"claims"`
		Mapped int `json:"mapped"`
	} `json:"intent"`
}

func statusOfTheRound(t *testing.T) roundStatus {
	t.Helper()
	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report roundStatus
	require.NoError(t, json.Unmarshal([]byte(printed), &report), printed)
	return report
}

func briefTheMovedHead(t *testing.T) {
	t.Helper()
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte("Load panics on a bad file.\n"), 0o600))
	_, err := runCLIPrinting(t, "brief", fixturePR, "--issue", fixtureIssue,
		"--intent-file", issue, "--repo", fixtureSlug)
	require.NoError(t, err)
}

// The brief that opens a new round leaves every earlier round's units and
// mapping lines where they were and adds the new round's units beside them, so
// a stale record's unit is still resolvable from state (§2.3.3 stamps each line
// with the round and head that wrote it). Every command of the new round still
// reads only its own lines (§9.3.5): `cr status` counts round 2's one unit and
// none of round 1's mapping, and `cr map record` replaces round 2's mapping
// while round 1's line stays.
//
// Measured before units-mapping-kept-per-round (QA D-S11-2): the brief left
// units.ndjson holding only round 2's line and mapping.ndjson empty.
func TestANewRoundKeepsEarlierRoundsUnitsAndMappingAndReadsOnlyItsOwn(t *testing.T) {
	layout, recorded, moved := aRoundTheHeadOutran(t)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	stamp := `"head":"` + recorded + `","round":1}`
	require.NoError(t, held.Write(state.FileClaims, []byte(
		`{"id":"`+fixtureIssue+`#c1","text":"Load panics on a bad file.","source":"acceptance",`+
			`"span":"panics on a bad file",`+stamp+"\n")))
	require.NoError(t, held.Write(state.FileMapping, []byte(
		`{"claim":"`+fixtureIssue+`#c1","unit":"u1",`+stamp+"\n")))
	require.NoError(t, held.Unlock())

	briefTheMovedHead(t)

	assert.Equal(t, []storedLine{
		{Round: 1, Head: recorded, ID: "u1", Path: "lib.go"},
		{Round: 2, Head: moved, ID: "u1", Path: "lib.go"},
	}, storedLines(t, layout, state.FileUnits),
		"round 1's unit stays as history and round 2's is added")
	assert.Equal(t, []storedLine{
		{Round: 1, Head: recorded, Claim: fixtureIssue + "#c1", Unit: "u1"},
	}, storedLines(t, layout, state.FileMapping),
		"round 1's mapping stays as history, and round 2 opens with none")

	opened := statusOfTheRound(t)
	assert.Equal(t, 2, opened.Round)
	assert.Equal(t, 1, opened.Coverage.Units, "round 2 formed one unit, and round 1's is not counted")
	assert.Equal(t, 1, opened.Intent.Claims, "the claim carried into round 2, and round 1's line is not counted")
	assert.Equal(t, 0, opened.Intent.Mapped, "round 1's mapping is not round 2's")

	pairs := filepath.Join(t.TempDir(), "mapping.ndjson")
	require.NoError(t, os.WriteFile(pairs, []byte(
		`{"claim":"`+fixtureIssue+`#c1","unit":"u1"}`+"\n"), 0o600))
	_, err = runCLIPrinting(t, "map", "record", fixturePR, pairs, "--repo", fixtureSlug)
	require.NoError(t, err)

	assert.Equal(t, []storedLine{
		{Round: 1, Head: recorded, Claim: fixtureIssue + "#c1", Unit: "u1"},
		{Round: 2, Head: moved, Claim: fixtureIssue + "#c1", Unit: "u1"},
	}, storedLines(t, layout, state.FileMapping),
		"cr map record replaces round 2's mapping and leaves round 1's")
	mapped := statusOfTheRound(t)
	assert.Equal(t, 1, mapped.Intent.Mapped, "round 2 reads its own pair once, and round 1's not at all")
	assert.Equal(t, 1, mapped.Coverage.Units)
}

// State an earlier version of cr left behind, whose closing brief had already
// cleared the units and mapping of every round before the current one, reads as
// it did and needs no migration: the next round is added beside the lines that
// are there, and `cr status` reads the new round alone.
func TestStateWhoseEarlierRoundsWereClearedStaysReadable(t *testing.T) {
	layout, recorded, moved := aRoundTheHeadOutran(t)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, Round: 2, Head: recorded,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":3,"end":5}],`+
			`"head":"`+recorded+`","round":2}`+"\n")))
	require.NoError(t, held.Write(state.FileMapping, nil))
	require.NoError(t, held.Unlock())

	briefTheMovedHead(t)

	assert.Equal(t, []storedLine{
		{Round: 2, Head: recorded, ID: "u1", Path: "lib.go"},
		{Round: 3, Head: moved, ID: "u1", Path: "lib.go"},
	}, storedLines(t, layout, state.FileUnits))
	assert.Empty(t, storedLines(t, layout, state.FileMapping))
	opened := statusOfTheRound(t)
	assert.Equal(t, 3, opened.Round)
	assert.Equal(t, 1, opened.Coverage.Units)
	assert.Equal(t, 0, opened.Intent.Mapped)
}
