package finding

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §9.1.1 over the whole of §9.1's table: every move the table lists, asked of a
// journal whose actor is the row's, leaves exactly one entry carrying the
// timestamp, the head SHA and the actor, and the two states it joined.
//
// The moves are drawn from table itself, so a row added to §9.1 is covered the
// day it is written. A move the table does not list is refused and leaves none.
func TestEveryTransitionOfTheTableLeavesExactlyOneJournalEntry(t *testing.T) {
	at := time.Date(2026, 9, 13, 2, 30, 0, 0, time.FixedZone("TRT", 3*60*60))
	const head = "9a8b7c6d5e4f30211220314f5e6d7c8b9a807162"
	moves := 0
	for triple := range allowed {
		journal := NewJournal(triple.actor, head, at)
		require.NoError(t, journal.Move("f1", triple.from, triple.to))
		entries := journal.entries
		require.Len(t, entries, 1, "%s by %s to %s", triple.from, triple.actor, triple.to)
		entry := entries[0]
		assert.Equal(t, "f1", entry.Record)
		assert.Equal(t, triple.to, entry.To)
		assert.Equal(t, triple.actor.String(), entry.Actor)
		assert.Equal(t, head, entry.Head)
		assert.True(t, at.Equal(entry.At), "the timestamp is the run's")
		assert.Equal(t, time.UTC, entry.At.Location(), "the timestamp is stored in UTC")
		if triple.from == Creation {
			assert.Nil(t, entry.From, "§9.1's first row starts from a new record, not a state")
		} else {
			require.NotNil(t, entry.From)
			assert.Equal(t, triple.from, Existing(*entry.From))
		}
		moves++
	}
	assert.Equal(t, 10, moves, "§9.1's six rows expand to ten moves")

	refused := NewJournal(ActorDraft, head, at)
	require.Error(t, refused.Move("f1", Existing(StatePosted), StateQueued))
	assert.Empty(t, refused.entries, "a refused move leaves no line")
}

// Write appends one line per move after the lines transitions.ndjson already
// holds, and a journal with nothing in it writes nothing at all.
func TestAJournalAppendsItsLinesAndWritesNothingWhenEmpty(t *testing.T) {
	l := state.New(t.TempDir())
	require.NoError(t, l.EnsurePR("acme", "api", 7))
	held, err := l.LockPR("acme", "api", 7)
	require.NoError(t, err)
	defer func() { require.NoError(t, held.Unlock()) }()

	require.NoError(t, NewJournal(ActorDraft, "abc", time.Now()).Write(held))
	none, err := state.ReadRecords[Transition](l, "acme", "api", 7, state.FileTransitions)
	require.NoError(t, err)
	assert.Empty(t, none)

	first := NewJournal(ActorRecord, "abc", time.Now())
	require.NoError(t, first.Move("f1", Creation, StateDraft))
	require.NoError(t, first.Write(held))
	second := NewJournal(ActorDraft, "abc", time.Now())
	require.NoError(t, second.Move("f1", Existing(StateDraft), StateQueued))
	require.NoError(t, second.Write(held))

	lines, err := state.ReadRecords[Transition](l, "acme", "api", 7, state.FileTransitions)
	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.Equal(t, "cr record", lines[0].Actor)
	assert.Equal(t, StateQueued, lines[1].To)
}
