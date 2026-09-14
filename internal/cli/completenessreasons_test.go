package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// rewriteStatusMeta rewrites the fixture's meta.json through change, under the
// lock, and leaves every other file as the fixture wrote it.
func rewriteStatusMeta(t *testing.T, change func(*state.Meta)) {
	t.Helper()
	layout := state.New(os.Getenv(state.HomeEnv))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	change(&meta)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())
}

// writeStatusFile replaces one of the fixture's per-PR files whole.
func writeStatusFile(t *testing.T, file, body string) {
	t.Helper()
	layout := state.New(os.Getenv(state.HomeEnv))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(file, []byte(body)))
	require.NoError(t, held.Unlock())
}

// §10.2.2's reason names every (unit, role) seat that lacks a current cell,
// not only how many rows lack one, and says so of a seat whose cell was filled
// for a hash the unit no longer carries.
//
// Measured on release QA before the fix (D-S04-1): the reason read "4 of 5
// unit(s) hold no complete row …" and named no unit, while §10.2.3's named its
// claim. statusHome's u2 holds no cell at all; here u1's convention cell is
// re-filed against an older hash.
func TestAnIncompleteRowReasonNamesEachSeatWithoutACurrentCell(t *testing.T) {
	statusHome(t)
	meta, err := state.New(os.Getenv(state.HomeEnv)).ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	var cells strings.Builder
	for role, hash := range map[string]string{"convention": "older", "correctness": "h1", "intent-coverage": "h1"} {
		cells.WriteString(`{"unit":"u1","role":"` + role + `","result":"pass","unit_hash":"` + hash +
			`","head":"` + meta.Head + `","round":1}` + "\n")
	}
	writeStatusFile(t, state.FileCoverage, cells.String())

	report := readCompleteness(t)

	require.NotEmpty(t, report.Completeness.Reasons)
	assert.Equal(t,
		"§10.2.2: 2 of 2 unit(s) hold no complete row of cells for all 3 active role(s) at their "+
			"current unit hash, missing u1/convention (filled for an earlier unit hash), "+
			"u2/convention, u2/correctness, u2/intent-coverage",
		report.Completeness.Reasons[0])
}

// While the intent axis is active, a round is not complete until its claims and
// its mapping are recorded, and the reason names which is missing; with the
// axis unavailable, neither is asked for.
//
// Measured on release QA before the fix (S04 spec gap): with the intent axis
// active and neither claims nor mapping ever recorded, `cr status` printed
// complete:true, because §10.2.3 holds over zero claims.
func TestARoundIsNotCompleteUntilItsIntentPassIsRecorded(t *testing.T) {
	cases := []struct {
		name      string
		unstamped bool
		noClaims  bool
		noKey     bool
		want      []string
	}{
		{name: "no mapping", unstamped: true, want: []string{
			"§4.6.5: the intent axis is active and this round has recorded no mapping: " +
				"record it with `cr map record`",
		}},
		{name: "no claims", noClaims: true, want: []string{
			"§4.6.5: the intent axis is active and this round has recorded no claims: " +
				"record them with `cr claims record`, then the mapping again with `cr map record`",
		}},
		{name: "neither", unstamped: true, noClaims: true, want: []string{
			"§4.6.5: the intent axis is active and this round has recorded neither its claims nor " +
				"its mapping: record the claims with `cr claims record`, then the mapping with `cr map record`",
		}},
		{name: "intent axis unavailable", unstamped: true, noClaims: true, noKey: true, want: []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			completeStatusHome(t)
			rewriteStatusMeta(t, func(m *state.Meta) {
				if c.unstamped {
					m.MappingRound, m.MappingHead = 0, ""
				}
				if c.noKey {
					m.IssueKey = ""
				}
			})
			if c.noClaims {
				writeStatusFile(t, state.FileClaims, "")
			}

			report := readCompleteness(t)

			assert.Equal(t, len(c.want) == 0, report.Completeness.Complete)
			assert.Equal(t, c.want, report.Completeness.Reasons)
		})
	}
}

// A cell citing a note that §3.6.6 retracted is named as needing
// re-evaluation and blocks completeness until it is filled again.
//
// Measured on release QA before the fix (S04 spec gap): after `cr note
// --remove`, `cr status` listed the cell under unstanding_notes and still
// printed complete:true.
func TestACellCitingARetractedNoteBlocksCompleteness(t *testing.T) {
	completeStatusHome(t)
	cited := appendStatusNote(t, "u1 only moves code the issue already has.")
	cellsFile := writeCellsInput(t,
		`{"unit":"u1","role":"intent-coverage","result":"pass","note_id":"`+cited+`"}`)
	_, err := runCLIPrinting(t, "cells", "record", fixturePR, cellsFile, "--repo", fixtureSlug)
	require.NoError(t, err)
	require.True(t, readCompleteness(t).Completeness.Complete, "the control: the note stands")

	_, err = runCLIPrinting(t, "note", "--remove", cited)
	require.NoError(t, err)
	report := readCompleteness(t)

	assert.False(t, report.Completeness.Complete)
	assert.Equal(t, []string{
		"§3.6.6: 1 cell(s) cite a note that no longer stands and need re-evaluation: " +
			"u1/intent-coverage (" + cited + " retracted)",
	}, report.Completeness.Reasons)

	refiled := writeCellsInput(t, `{"unit":"u1","role":"intent-coverage","result":"pass"}`)
	_, err = runCLIPrinting(t, "cells", "record", fixturePR, refiled, "--repo", fixtureSlug)
	require.NoError(t, err)
	assert.True(t, readCompleteness(t).Completeness.Complete,
		"filling the cell again without the note is the re-evaluation")
}

// cr cells record refuses with exit 1 a note_id naming no note of the round's
// issue key, and one §3.6.6 has retracted, and stores nothing.
//
// Measured on release QA before the fix (cell-09): a note_id of `CR-9#n7`,
// which no store held, was accepted with exit 0.
func TestCellsRecordRefusesANoteIDThatDoesNotStand(t *testing.T) {
	completeStatusHome(t)
	retracted := appendStatusNote(t, "A note withdrawn before it was cited.")
	_, err := runCLIPrinting(t, "note", "--remove", retracted)
	require.NoError(t, err)
	layout := state.New(os.Getenv(state.HomeEnv))
	before, err := os.ReadFile(layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileCoverage))
	require.NoError(t, err)

	for id, problem := range map[string]string{
		"CR-9#n7": `names "CR-9#n7", which is no note of issue ` + fixtureIssue + `; §4.5.5 cites a note ` +
			`of §3.6 for the round's issue key, and ` + "`cr context " + fixtureIssue + "`" + ` lists them`,
		fixtureIssue + "#n9": `names "` + fixtureIssue + `#n9", which is no note of issue ` + fixtureIssue +
			`; §4.5.5 cites a note of §3.6 for the round's issue key, and ` +
			"`cr context " + fixtureIssue + "`" + ` lists them`,
		retracted: `names "` + retracted + `", which §3.6.6 has retracted, so it explains nothing; ` +
			`cite a note that stands, or drop the field`,
	} {
		cellsFile := writeCellsInput(t,
			`{"unit":"u2","role":"intent-coverage","result":"pass","note_id":"`+id+`"}`)
		_, err := runCLIPrinting(t, "cells", "record", fixturePR, cellsFile, "--repo", fixtureSlug)
		require.Error(t, err, id)
		assert.Equal(t, ExitValidation, exitCodeFor(err), id)
		assert.Equal(t, cellsFile+" line 1: note_id "+problem, err.Error(), id)
	}
	after, err := os.ReadFile(layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileCoverage))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "a refused file stores no cell")
}

// appendStatusNote records a note for the fixture's issue key and returns its
// id.
func appendStatusNote(t *testing.T, body string) string {
	t.Helper()
	added, err := note.Append(state.New(os.Getenv(state.HomeEnv)), fixtureIssue, body,
		note.SourceChat, fixturePRNumber, time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	return added.ID
}

// writeCellsInput writes cell lines to a file of their own for `cr cells
// record`.
func writeCellsInput(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cells.ndjson")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	return path
}
