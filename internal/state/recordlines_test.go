package state

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RecordLines and DecodeStamped count the same lines.
//
// §6.2.3's rejection is raised after the decode, over a slice of records that
// carries no line numbers of its own, and it has to name the line the user must
// open exactly as §6.1.3's rejections do. Its caller indexes the two slices
// together, so what makes that total rather than lucky is here: one function
// decides which lines carry a record, the decode appends exactly one record for
// each line it names, and the numbers are the same numbers a refusal inside the
// decode would have printed.
//
// The body carries blank lines in every position that can shift a count — a
// leading one, a run in the middle, a whitespace-only one, and a trailing
// newline — because the whole reason the numbers cannot be the slice index is
// that DecodeStamped skips those and keeps counting them.
func TestRecordLinesAgreesWithTheDecodeItNumbers(t *testing.T) {
	body := []byte("\n{\"id\":\"f1\"}\n\n\n   \n{\"id\":\"f2\"}\n{\"id\":\"f3\"}\n\n")

	numbered := RecordLines(body)
	assert.Equal(t, []int{2, 6, 7}, numbered,
		"the one-based line each record sits on, counting the blank ones skipped to reach it")

	var visited []int
	decoded, err := DecodeStamped[stampedRecord](
		"merged.ndjson", body,
		func(line int, _ map[string]json.RawMessage, _ *stampedRecord) error {
			// The number the decode would name in a refusal, which
			// is the number the caller has to be able to recover
			// once the decode has returned.
			visited = append(visited, line)
			return nil
		})

	require.NoError(t, err)
	assert.Equal(t, numbered, visited,
		"the decode numbers the lines RecordLines names, in that order")
	assert.Len(t, decoded, len(numbered),
		"exactly one record for each line RecordLines names, so the two index together")
}

// A line the decode refuses returns no records at all, which is the other half
// of the indexing being total: a caller never holds a record slice shorter than
// the line slice, because a decode that produced one returned an error instead.
func TestARefusedLineLeavesNoRecordsToIndex(t *testing.T) {
	body := []byte("{\"id\":\"f1\"}\n\n{\"id\":\"f2\"}\n{\"id\":\"f3\"}\n")
	require.Equal(t, []int{1, 3, 4}, RecordLines(body))

	decoded, err := DecodeStamped[stampedRecord](
		"merged.ndjson", body,
		func(line int, _ map[string]json.RawMessage, _ *stampedRecord) error {
			if line == 3 {
				return &ReservedFieldError{File: "merged.ndjson", Line: line, Field: "id"}
			}
			return nil
		})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 3", "the refusal names the line the user must open")
	assert.Empty(t, decoded, "and returns nothing to index against")
}
