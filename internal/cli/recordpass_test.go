package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// recordedHomeWithCells is recordedHome's round with §4.5.1's active set
// settled, so `cr cells record` accepts cells from the two roles the fixture
// names, and with f1 already recorded on u2 so findings.ndjson has bytes to
// compare.
func recordedHomeWithCells(t *testing.T) state.Layout {
	t.Helper()
	layout := recordedHome(t)
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: recordOwner, Repo: recordRepo, PR: recordPRNum,
		Round: recordRound, Head: recordHead, MappingRound: recordRound, MappingHead: recordHead,
		ActiveRoles: []string{"convention", "correctness"},
	}))
	require.NoError(t, held.Unlock())
	_, err = runRecord(t, recordPR, writeRecordFile(t, "first.ndjson", aRecord("f1", "u2")),
		"--repo", recordSlug)
	require.NoError(t, err)
	return layout
}

// recordPassCells hands `cr cells record` the cells named, against the record
// fixture's pull request.
func recordPassCells(t *testing.T, lines ...string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cells.ndjson")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	require.NoError(t, runCLI(t, "cells", "record", recordPR, path, "--repo", recordSlug))
}

// The order cell-record-consistency left unchecked: a pass cell recorded first,
// then a record from that role on that unit. The record is refused with exit 1
// naming the record, the unit, the role and the cell, and findings.ndjson is
// byte-identical afterwards — not even the good record above it is stored.
func TestARecordMeetingAPassCellOfItsRoleOnItsUnitIsRefused(t *testing.T) {
	layout := recordedHomeWithCells(t)
	recordPassCells(t, `{"unit":"u1","role":"correctness","result":"pass"}`)
	findings := layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings)
	before, err := os.ReadFile(findings)
	require.NoError(t, err)
	require.NotEmpty(t, before)

	_, err = runRecord(t, recordPR,
		writeRecordFile(t, "second.ndjson", aRecord("f2", "u2"), aRecord("f3", "u1")),
		"--repo", recordSlug)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§11.2 codes a refused record 1")
	assert.Equal(t, 2, rejected.Line)
	assert.Equal(t, "role", rejected.Field)
	for _, named := range []string{`"f3"`, `unit "u1"`, `role "correctness"`, "pass cell"} {
		assert.Contains(t, err.Error(), named, "the refusal names the record, the unit, the role and the cell")
	}
	after, err := os.ReadFile(findings)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "a refused file writes nothing, not even its good f2")
}

// The seat is the pair, and only a pass contradicts: a pass from another role on
// the record's unit and a finding cell at the record's own seat both stand
// beside it.
func TestARecordBesideAPassAtAnotherSeatOrAFindingCellIsAccepted(t *testing.T) {
	layout := recordedHomeWithCells(t)
	recordPassCells(t,
		`{"unit":"u1","role":"convention","result":"pass"}`,
		`{"unit":"u1","role":"correctness","result":"finding"}`)

	_, err := runRecord(t, recordPR, writeRecordFile(t, "second.ndjson", aRecord("f2", "u1")),
		"--repo", recordSlug)

	require.NoError(t, err)
	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	assert.Len(t, stored, 2)
}
