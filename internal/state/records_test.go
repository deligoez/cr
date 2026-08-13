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

// §2.3.3's head and round belong to the writer. A record arriving with values
// of its own does not keep them, which is what lets a later command reject the
// agent that supplied them without every call site remembering to check.
func TestTheWriterOwnsHeadAndRound(t *testing.T) {
	l := lockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)

	records := []*stampedRecord{
		{ID: "c1"},
		{ID: "c2", Stamp: Stamp{Head: "deadbee", Round: 99}},
	}
	require.NoError(t, WriteStamped(held, FileClaims, Stamp{Head: "0f1e2d3", Round: 2}, records))
	require.NoError(t, held.Unlock())

	got, err := ReadRecords[stampedRecord](l, "acme", "web", 42, FileClaims)
	require.NoError(t, err)
	assert.Equal(t, []stampedRecord{
		{Stamp: Stamp{Head: "0f1e2d3", Round: 2}, ID: "c1"},
		{Stamp: Stamp{Head: "0f1e2d3", Round: 2}, ID: "c2"},
	}, got)
}

// §2.3.3 names exactly eight files. Which writer a file takes follows from that
// list and not from the caller, so a stamped file cannot be written unstamped
// and an unstamped one cannot acquire the pair by accident.
func TestAFileIsWrittenThroughTheWriterItsSchemaRequires(t *testing.T) {
	l := lockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	defer func() { assert.NoError(t, held.Unlock()) }()

	at := Stamp{Head: "0f1e2d3", Round: 1}
	for _, name := range []string{
		FileClaims, FileUnits, FileMapping, FileFindings,
		FileProbes, FileRuns, FileIntentGaps, FileCoverage,
	} {
		assert.Error(t, WriteRecords(held, name, []plainRecord{}), name)
		assert.NoError(t, WriteStamped(held, name, at, []*stampedRecord{}), name)
	}
	for _, name := range []string{FilePostedIndex, FileThreads, FileTransitions, FileWaivers} {
		assert.NoError(t, WriteRecords(held, name, []plainRecord{}), name)
		assert.Error(t, WriteStamped(held, name, at, []*stampedRecord{}), name)
	}
}

