package state

import (
	"encoding/json"
	"path/filepath"
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

// A line that does not decode is named by its number, counted over the file as
// written so a blank line does not shift what the user is told to open.
func TestAMalformedRecordNamesItsLine(t *testing.T) {
	l := lockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, held.Write(FileThreads, []byte("{\"id\":\"t1\"}\n\nnot json\n")))
	require.NoError(t, held.Unlock())

	_, err = ReadRecords[plainRecord](l, "acme", "web", 42, FileThreads)
	require.Error(t, err)
	assert.Contains(t, err.Error(), l.PRFile("acme", "web", 42, FileThreads))
	assert.Contains(t, err.Error(), "line 3")
}

// A record cr cannot encode is reported by its position, and the file it was
// bound for keeps what it had: the whole document is built before any of it is
// published.
func TestAnUnencodableRecordLeavesTheFileUntouched(t *testing.T) {
	l := lockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, WriteRecords(held, FileThreads, []plainRecord{{ID: "t1"}}))

	err = WriteRecords(held, FileThreads, []any{plainRecord{ID: "t2"}, make(chan int)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "record 2 of threads.ndjson")
	require.NoError(t, held.Unlock())

	body, err := l.ReadPR("acme", "web", 42, FileThreads)
	require.NoError(t, err)
	assert.Equal(t, "{\"id\":\"t1\"}\n", string(body))
}

// §2.3.3's pair is written by cr, so a record that supplied either was written
// by the agent and is refused by the line and the field, exactly as §6.1.4
// refuses a computed field. Presence on the wire is what counts: a zero round
// and an empty head are values the agent chose, and a decoded struct could not
// tell them from a field that was never there.
func TestAnAgentMayNotSupplyHeadOrRound(t *testing.T) {
	for _, tc := range []struct {
		name  string
		line  string
		field string
	}{
		{name: "a head of its own", line: `{"id":"c2","head":"deadbee"}`, field: "head"},
		{name: "a round of its own", line: `{"id":"c2","round":9}`, field: "round"},
		{name: "an empty head", line: `{"id":"c2","head":""}`, field: "head"},
		{name: "a zero round", line: `{"id":"c2","round":0}`, field: "round"},
		{name: "a null head", line: `{"id":"c2","head":null}`, field: "head"},
		{name: "both, named by the first", line: `{"id":"c2","round":9,"head":"x"}`, field: "head"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeStamped[stampedRecord](
				FileClaims, []byte("{\"id\":\"c1\"}\n\n"+tc.line+"\n"), nil,
			)
			var reserved *ReservedFieldError
			require.ErrorAs(t, err, &reserved)
			assert.Equal(t, FileClaims, reserved.File)
			assert.Equal(t, 3, reserved.Line)
			assert.Equal(t, tc.field, reserved.Field)
			assert.Equal(
				t,
				"claims.ndjson line 3: "+tc.field+" is written by cr and must not be supplied",
				err.Error(),
			)
		})
	}
}

// encoding/json binds an object key to a struct field whatever its letter case,
// so `"Head"` sets Stamp.Head exactly as `"head"` does. The fence reads a key
// the way the decode binds it, or a line would get past §2.3.3 by capitalising
// one letter; the refusal names the field by §2.3.3's own spelling.
func TestHeadAndRoundAreRefusedUnderAnyKeyCase(t *testing.T) {
	for _, tc := range []struct{ line, field string }{
		{line: `{"id":"c2","Head":"deadbee"}`, field: "head"},
		{line: `{"id":"c2","HEAD":""}`, field: "head"},
		{line: `{"id":"c2","rOuNd":9}`, field: "round"},
	} {
		t.Run(tc.line, func(t *testing.T) {
			_, err := DecodeStamped[stampedRecord](
				FileClaims, []byte("{\"id\":\"c1\"}\n\n"+tc.line+"\n"), nil,
			)
			var reserved *ReservedFieldError
			require.ErrorAs(t, err, &reserved)
			assert.Equal(t, &ReservedFieldError{File: FileClaims, Line: 3, Field: tc.field}, reserved)
		})
	}
}

// FoldedFields keys a line's fields the way encoding/json binds them to a
// struct: one key per field whatever case each spelling used, folding the
// Unicode letters the decode folds too (U+017F long s is an s, U+212A Kelvin
// sign a k), and holding the value the decode keeps, which is the last one
// written. A check reading an earlier spelling would judge a value the record
// does not hold.
func TestFoldedFieldsKeyALineTheWayTheDecodeBindsIt(t *testing.T) {
	for _, tc := range []struct {
		line string
		id   string
	}{
		{line: `{"ID":"c1","id":"c2","I\u017f":"x","\u212aind":"y","Note_Id":null}`, id: "c2"},
		{line: `{"id":"c2","I\u017f":"x","\u212aind":"y","Note_Id":null,"iD":"c1"}`, id: "c1"},
	} {
		t.Run(tc.line, func(t *testing.T) {
			fields, err := FoldedFields([]byte(tc.line))
			require.NoError(t, err)
			assert.Equal(t, map[string]json.RawMessage{
				"id":      json.RawMessage(`"` + tc.id + `"`),
				"is":      json.RawMessage(`"x"`),
				"kind":    json.RawMessage(`"y"`),
				"note_id": json.RawMessage(`null`),
			}, fields)

			var decoded plainRecord
			require.NoError(t, json.Unmarshal([]byte(tc.line), &decoded))
			assert.Equal(t, tc.id, decoded.ID, "the decode keeps the spelling the fold keeps")
		})
	}

	nothing, err := FoldedFields([]byte("null"))
	require.NoError(t, err)
	assert.Nil(t, nothing, "null is no object and no fields, exactly as json.Unmarshal reads it")

	_, err = FoldedFields([]byte("[1, 2]"))
	assert.Error(t, err, "a line that is not an object has no fields to key")
}

// Decoding is the road from an agent's file to the writer: what it returns is
// what WriteStamped stamps. An empty file is no records rather than a null
// slice (§12.3).
func TestDecodedRecordsAreStampedOnTheWayOut(t *testing.T) {
	l := lockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)

	records, err := DecodeStamped[stampedRecord](
		FileClaims, []byte("{\"id\":\"c1\"}\n{\"id\":\"c2\"}\n"), nil,
	)
	require.NoError(t, err)
	require.NoError(t, WriteStamped(held, FileClaims, Stamp{Head: "0f1e2d3", Round: 2}, records))
	require.NoError(t, held.Unlock())

	got, err := ReadRecords[stampedRecord](l, "acme", "web", 42, FileClaims)
	require.NoError(t, err)
	assert.Equal(t, []stampedRecord{
		{Stamp: Stamp{Head: "0f1e2d3", Round: 2}, ID: "c1"},
		{Stamp: Stamp{Head: "0f1e2d3", Round: 2}, ID: "c2"},
	}, got)

	empty, err := DecodeStamped[stampedRecord](FileClaims, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, []*stampedRecord{}, empty)
}

// A line cr cannot decode is named by its number, counted over the file as the
// agent handed it in, whether the line is no record at all or a record whose
// field has the wrong type. Neither is a supplied-field rejection.
func TestAnUndecodableAgentLineIsNamedByItsNumber(t *testing.T) {
	for _, tc := range []struct{ name, line string }{
		{name: "not a JSON object", line: "[1, 2]"},
		{name: "a field of the wrong type", line: `{"id":7}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeStamped[stampedRecord](
				FileClaims, []byte("{\"id\":\"c1\"}\n\n"+tc.line+"\n"), nil,
			)
			require.Error(t, err)
			var reserved *ReservedFieldError
			assert.NotErrorAs(t, err, &reserved)
			assert.Contains(t, err.Error(), "claims.ndjson line 3")
		})
	}
}

// §9.3.5 through the writer: an append adds a round's records and leaves what
// the file already held exactly as it was.
//
// findings.ndjson is the file this is about. §2.3 has it hold every finding and
// question in every state, so a round adds to it, and §9.3.5 leaves earlier
// rounds intact. WriteStamped cannot be used for that: it stamps every record
// it is handed, so reading the file back and rewriting it through WriteStamped
// would carry every earlier round into the head being recorded now — the first
// round's records would claim to have been produced against a commit that did
// not exist when they were written.
//
// The first append lands on a file that is not there yet, which is what a state
// directory opened by LockPR alone has: an absent file holds as many records as
// an empty one, and the append creates it.
func TestAnAppendLeavesEarlierRoundsAsTheyWere(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, l.Init())
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)

	first := Stamp{Head: "0f1e2d3", Round: 1}
	second := Stamp{Head: "4c5b6a7", Round: 2}
	require.NoError(t, AppendStamped(held, FileFindings, first, []*stampedRecord{{ID: "f1"}}))
	require.NoError(t, AppendStamped(held, FileFindings, second,
		[]*stampedRecord{{ID: "f2", Stamp: first}, {ID: "f3"}}))

	// A file §2.3.3 does not list carries no pair at all, and is refused
	// here for the reason WriteStamped refuses it.
	assert.Error(t, AppendStamped(held, FileThreads, first, []*stampedRecord{}))
	require.NoError(t, held.Unlock())

	got, err := ReadRecords[stampedRecord](l, "acme", "web", 42, FileFindings)
	require.NoError(t, err)
	assert.Equal(t, []stampedRecord{
		{Stamp: first, ID: "f1"},
		{Stamp: second, ID: "f2"},
		{Stamp: second, ID: "f3"},
	}, got, "the first round keeps the head it was recorded against, and the second stamps its own")
}
