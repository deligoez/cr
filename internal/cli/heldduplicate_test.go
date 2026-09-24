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

// §6.4.5 through the command: a record the round already holds in `draft`
// absorbs a later file's record at the same path, line and class, which is
// stored as its duplicate rather than as a second comment on the line. A record
// on another line is stored as ever.
//
// This is the situation §9.3.4's carry creates: the record is already in the
// round, and the round's fan-out reviews its unit again.
func TestARecordTheRoundAlreadyHoldsAbsorbsItsReRaise(t *testing.T) {
	layout := recordedHome(t)
	_, err := runRecord(t, recordPR, writeRecordFile(t, "first.ndjson", aRecord("f1", "u1")), "--repo", recordSlug)
	require.NoError(t, err)

	again := aRecord("f2", "u1")
	elsewhere := aRecord("f3", "u2")
	_, err = runRecord(t, recordPR, writeRecordFile(t, "second.ndjson", again, elsewhere), "--repo", recordSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 3)
	byID := map[string]finding.Finding{}
	for _, held := range stored {
		byID[held.ID] = held
	}
	assert.Equal(t, finding.StateDraft, byID["f1"].State, "the held record stays the representative")
	assert.Equal(t, finding.StateDuplicate, byID["f2"].State, "§6.4.5: the re-raise is a duplicate")
	assert.Equal(t, "f1", byID["f2"].DuplicateOf)
	assert.Equal(t, finding.StateDraft, byID["f3"].State, "another line is another record")
	assert.Empty(t, byID["f3"].DuplicateOf)
}

// §6.4.5 holds a `queued` record as it holds a `draft` one: the reviewer has
// approved it, and a re-raise beside it would be a second comment on its line.
func TestARecordHeldQueuedAbsorbsItsReRaise(t *testing.T) {
	layout := recordedHome(t)
	_, err := runRecord(t, recordPR, writeRecordFile(t, "first.ndjson", aRecord("f1", "u1")), "--repo", recordSlug)
	require.NoError(t, err)
	path := filepath.Join(layout.PRDir(recordOwner, recordRepo, recordPRNum), state.FileFindings)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(body), `"state":"draft"`))
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings,
		[]byte(strings.Replace(string(body), `"state":"draft"`, `"state":"queued"`, 1))))
	require.NoError(t, held.Unlock())

	_, err = runRecord(t, recordPR, writeRecordFile(t, "second.ndjson", aRecord("f2", "u1")), "--repo", recordSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	assert.Equal(t, finding.StateDuplicate, stored[1].State, "§6.4.5: the re-raise of a queued record is a duplicate")
	assert.Equal(t, "f1", stored[1].DuplicateOf)
}
