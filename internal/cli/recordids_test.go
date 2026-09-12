package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// §6.1 makes a record id stable for the life of the pull request: a batch
// carrying f1 while a stored record already holds f1 is refused with exit 1,
// naming the id, the line and the stored record, and findings.ndjson is
// byte-identical afterwards.
func TestARecordIDAlreadyStoredIsRefusedAndNothingIsWritten(t *testing.T) {
	layout := recordedHome(t)
	_, err := runRecord(t, recordPR, writeRecordFile(t, "first.ndjson", aRecord("f1", "u1")),
		"--repo", recordSlug)
	require.NoError(t, err)
	findings := layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings)
	before, err := os.ReadFile(findings)
	require.NoError(t, err)

	_, err = runRecord(t, recordPR,
		writeRecordFile(t, "second.ndjson", aRecord("f2", "u1"), aRecord("f1", "u1")),
		"--repo", recordSlug)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, 2, rejected.Line)
	assert.Equal(t, "id", rejected.Field)
	assert.Contains(t, rejected.Problem, `"f1" is already held by the record stored for this pull request in round`)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§6.1.3's refusals exit 1")
	after, err := os.ReadFile(findings)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "a refused batch writes nothing, not even its fresh f2")
}

// An id repeated inside one batch is refused with exit 1, naming both lines,
// and no record of the batch is stored.
func TestARecordIDRepeatedWithinOneBatchIsRefused(t *testing.T) {
	layout := recordedHome(t)

	_, err := runRecord(t, recordPR,
		writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1"), aRecord("f2", "u1"), aRecord("f1", "u1")),
		"--repo", recordSlug)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, 3, rejected.Line)
	assert.Equal(t, "id", rejected.Field)
	assert.Contains(t, rejected.Problem, `"f1" repeats the id of line 1`)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	assert.Empty(t, stored, "a refused file stores none of its records")
}
