package cli

import (
	"os"
	"regexp"
	"strconv"
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

// nextFreeIDIn reads the id a refusal names as free, whole: the id the sentence
// ends on, so f1 is never read out of f10.
func nextFreeIDIn(t *testing.T, problem string) string {
	t.Helper()
	named := regexp.MustCompile(`; the next free id (?:in f\d+\.\.f\d+, the block cr review gave the [a-z-]+ prompt on unit u\d+, )?is (f\d+)$`).
		FindStringSubmatch(problem)
	require.Len(t, named, 2, "the refusal names the next free id at its end: %s", problem)
	return named[1]
}

// Two records of one file sharing f1, both outside their prompts' blocks, are
// refused on the second line, naming the free id inside that record's own block
// — the correctness prompt on u2, f301..f400 — over the store and the whole
// file: a stored f301 and a later line's f302 are both spent, so the id named
// is f303, where the store alone would give f302.
func TestARepeatedRecordIDIsRefusedNamingTheNextFreeID(t *testing.T) {
	recordedHome(t)
	_, err := runRecord(t, recordPR, writeRecordFile(t, "first.ndjson", aRecord("f301", "u2")),
		"--repo", recordSlug)
	require.NoError(t, err)

	_, err = runRecord(t, recordPR,
		writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1"), aRecord("f1", "u2"), aRecord("f302", "u1")),
		"--repo", recordSlug)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, 2, rejected.Line)
	assert.Equal(t, "id", rejected.Field)
	assert.Equal(t, "f303", nextFreeIDIn(t, rejected.Problem))
	assert.Equal(t, ExitValidation, exitCodeFor(err))
}

// A record of a later round reusing an id an earlier round's record holds is
// refused naming a free id of its own prompt's block: the fixture's round spent
// f1 and f6, so the next round's blocks start past f6, and the correctness
// prompt on u1 — the third of two units times the shipped roles in order — is
// given f207..f306; the id named is f207.
func TestALaterRoundRecordReusingAnEarlierRoundIDIsRefusedNamingTheNextFreeID(t *testing.T) {
	layout := recordedHome(t)
	_, err := runRecord(t, recordPR,
		writeRecordFile(t, "earlier.ndjson", aRecord("f1", "u1"), aRecord("f6", "u2")),
		"--repo", recordSlug)
	require.NoError(t, err)
	openNextRound(t, layout)

	_, err = runRecord(t, recordPR,
		writeRecordFile(t, "later.ndjson", aRecord("f1", "u1"), aRecord("f3", "u2")),
		"--repo", recordSlug)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, 1, rejected.Line)
	assert.Equal(t, "id", rejected.Field)
	heldIn := regexp.MustCompile(`^"f1" is already held by the record stored for this pull request in round ([0-9]+) at head `).
		FindStringSubmatch(rejected.Problem)
	require.Len(t, heldIn, 2, rejected.Problem)
	assert.Equal(t, strconv.Itoa(recordRound), heldIn[1], "the holder is the earlier round's record")
	assert.Equal(t, "f207", nextFreeIDIn(t, rejected.Problem))
	assert.Equal(t, ExitValidation, exitCodeFor(err))
}
