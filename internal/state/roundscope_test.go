package state

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundsRecorded writes one line per round into name, so a round-scoped write
// has history to leave alone. The lines go in as bytes rather than through a
// writer: the round is the whole of what earlierRounds reads, and a fixture
// built through ReplaceStamped would already depend on earlierRounds.
func roundsRecorded(t *testing.T, k *Lock, name string, rounds ...int) {
	t.Helper()
	var body []byte
	for _, round := range rounds {
		body = fmt.Appendf(body, "{\"id\":\"r%d\",\"head\":\"0f1e2d3\",\"round\":%d}\n", round, round)
	}
	require.NoError(t, k.Write(name, body))
}

// §3.3.1 replaces claims.ndjson and §9.3.5 scopes the replacement: the current
// round's records go, every other round's stay.
//
// The fixture holds two earlier rounds rather than one, so a writer that kept
// only the line nearest the replacement would still fail, and the round being
// replaced sits between them, so keeping "everything before" is not enough
// either — earlierRounds has to take out one round and leave both neighbours.
func TestReplaceStampedRewritesOneRoundAndLeavesTheOthers(t *testing.T) {
	l := unlockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	roundsRecorded(t, held, FileClaims, 1, 2, 3)

	at := Stamp{Head: "9a8b7c6", Round: 2}
	require.NoError(t, ReplaceStamped(held, FileClaims, at, []*stampedRecord{{ID: "c9"}}))
	require.NoError(t, held.Unlock())

	got, err := ReadRecords[stampedRecord](l, "acme", "web", 42, FileClaims)
	require.NoError(t, err)
	assert.Equal(t, []stampedRecord{
		{Stamp: Stamp{Head: "0f1e2d3", Round: 1}, ID: "r1"},
		{Stamp: Stamp{Head: "0f1e2d3", Round: 3}, ID: "r3"},
		{Stamp: at, ID: "c9"},
	}, got, "§9.3.5: the round is replaced, and earlier rounds are left intact")
}

// §3.3.1 clears mapping.ndjson when claims are recorded, and §9.3.5 scopes that
// clearing the same way: the file is emptied of this round and of nothing else.
//
// The file is read back as bytes as well as records, because "cleared" has two
// readings and only one of them is right. A writer that truncated the file
// would satisfy any assertion counting this round's records, and would have
// destroyed exactly the history §9.3.5 protects.
func TestClearStampedEmptiesOneRoundAndNotTheFile(t *testing.T) {
	l := unlockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	roundsRecorded(t, held, FileMapping, 1, 2)

	require.NoError(t, ClearStamped(held, FileMapping, 2))
	require.NoError(t, held.Unlock())

	body, err := l.ReadPR("acme", "web", 42, FileMapping)
	require.NoError(t, err)
	assert.Equal(t, "{\"id\":\"r1\",\"head\":\"0f1e2d3\",\"round\":1}\n", string(body),
		"round 1's line is carried through byte for byte, and round 2's is gone")
}

// A round-scoped write reads the file it is about to rewrite, so a line it
// cannot parse stops the write and names the line.
//
// Refusing is the only safe answer. The round is what decides whether a line
// survives, and a line whose round cannot be read has no answer: keeping it
// would leave this round's records in a file documented as replaced, and
// dropping it would delete a record §9.3.5 calls history on the strength of a
// parse failure. The write is refused whole instead, and the file is left
// exactly as it was for the user to open at the line named.
func TestARoundScopedWriteRefusesAStoreItCannotRead(t *testing.T) {
	l := unlockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	defer func() { assert.NoError(t, held.Unlock()) }()

	corrupt := "{\"id\":\"r1\",\"head\":\"0f1e2d3\",\"round\":1}\n\n{\"id\":\"r2\",\n"
	require.NoError(t, held.Write(FileClaims, []byte(corrupt)))

	at := Stamp{Head: "9a8b7c6", Round: 2}
	err = ReplaceStamped(held, FileClaims, at, []*stampedRecord{{ID: "c9"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), FileClaims, "the refusal names the file")
	assert.Contains(t, err.Error(), "line 3",
		"and the one-based line, counting the blank one, so the number opens the line")

	body, err := l.ReadPR("acme", "web", 42, FileClaims)
	require.NoError(t, err)
	assert.Equal(t, corrupt, string(body), "and nothing was written over it")
}

// keyedRecord is a §2.3.3 record keyed by more than its round: coverage.ndjson
// holds one cell per (unit, role), which is the shape ReplaceStampedKeys was
// written for.
type keyedRecord struct {
	Stamp
	Unit   string `json:"unit"`
	Role   string `json:"role"`
	Result string `json:"result"`
}

// cellsRecorded writes one line per (unit, role) into name, as bytes, so a
// keyed write has cells of this round and of another to leave alone.
func cellsRecorded(t *testing.T, k *Lock, name string, at ...keyedRecord) {
	t.Helper()
	var body []byte
	for _, cell := range at {
		body = fmt.Appendf(body,
			"{\"head\":\"0f1e2d3\",\"round\":%d,\"unit\":%q,\"role\":%q,\"result\":%q}\n",
			cell.Round, cell.Unit, cell.Role, cell.Result)
	}
	require.NoError(t, k.Write(name, body))
}

// §4.5.6 replaces the current round's cell for each (unit, role) the file names
// and leaves every other cell untouched.
//
// The fixture is built so that each half of that sentence can fail on its own.
// One cell of this round is named and replaced; a second cell of this round at
// the same unit under another role is not named, and a third at the same role
// under another unit is not either — so a writer keyed on the unit alone or on
// the role alone deletes one of them. A cell of another round sits alongside,
// which is §9.3.5's half: a keyed write is still round-scoped, and a stored
// cell at a named key in an earlier round is history rather than a cell to
// replace.
func TestReplaceStampedKeysRewritesTheNamedKeysAndNoOthers(t *testing.T) {
	l := unlockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	cellsRecorded(t, held, FileCoverage,
		keyedRecord{Stamp: Stamp{Round: 1}, Unit: "u1", Role: "correctness", Result: "pass"},
		keyedRecord{Stamp: Stamp{Round: 2}, Unit: "u1", Role: "correctness", Result: "pass"},
		keyedRecord{Stamp: Stamp{Round: 2}, Unit: "u1", Role: "convention", Result: "pass"},
		keyedRecord{Stamp: Stamp{Round: 2}, Unit: "u2", Role: "correctness", Result: "pass"},
	)

	at := Stamp{Head: "9a8b7c6", Round: 2}
	require.NoError(t, ReplaceStampedKeys(held, FileCoverage, at,
		[]*keyedRecord{{Unit: "u1", Role: "correctness", Result: "finding"}},
		[]string{"unit", "role"}))
	require.NoError(t, held.Unlock())

	got, err := ReadRecords[keyedRecord](l, "acme", "web", 42, FileCoverage)
	require.NoError(t, err)
	assert.Equal(t, []keyedRecord{
		{Stamp: Stamp{Head: "0f1e2d3", Round: 1}, Unit: "u1", Role: "correctness", Result: "pass"},
		{Stamp: Stamp{Head: "0f1e2d3", Round: 2}, Unit: "u1", Role: "convention", Result: "pass"},
		{Stamp: Stamp{Head: "0f1e2d3", Round: 2}, Unit: "u2", Role: "correctness", Result: "pass"},
		{Stamp: at, Unit: "u1", Role: "correctness", Result: "finding"},
	}, got, "§4.5.6: only the named (unit, role) of the current round is replaced")
}
