package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stampedRecord stands in for the record types §2.3.3 names, none of which
// exist yet. It carries head and round the way they will: by embedding Stamp.
type stampedRecord struct {
	Stamp
	ID string `json:"id"`
}

// plainRecord stands in for a record of a file §2.3.3 does not list.
type plainRecord struct {
	ID string `json:"id"`
}

// NDJSON is one JSON document per line, and an empty file is no records rather
// than a null slice (§12.3).
func TestRecordsAreOneDocumentPerLine(t *testing.T) {
	l := lockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, WriteRecords(held, FileThreads, []plainRecord{{ID: "t1"}, {ID: "t2"}}))
	require.NoError(t, held.Unlock())

	body, err := l.ReadPR("acme", "web", 42, FileThreads)
	require.NoError(t, err)
	assert.Equal(t, "{\"id\":\"t1\"}\n{\"id\":\"t2\"}\n", string(body))

	got, err := ReadRecords[plainRecord](l, "acme", "web", 42, FileThreads)
	require.NoError(t, err)
	assert.Equal(t, []plainRecord{{ID: "t1"}, {ID: "t2"}}, got)

	empty, err := ReadRecords[plainRecord](l, "acme", "web", 42, FileTransitions)
	require.NoError(t, err)
	assert.Equal(t, []plainRecord{}, empty, "an empty file is no records, never a null slice")
}

