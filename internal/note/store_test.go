package note

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// storeRoot returns a layout with the global tree already created.
func storeRoot(t *testing.T) state.Layout {
	t.Helper()
	l := state.New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, l.Init())
	return l
}

// stored decodes the notes on disk for one issue key, which is what a later
// round and `cr context` will read rather than the value Append returned.
func stored(t *testing.T, l state.Layout, issueKey string) []Note {
	t.Helper()
	body, err := os.ReadFile(l.ContextFile(issueKey))
	require.NoError(t, err)

	notes := make([]Note, 0)
	for _, line := range strings.Split(strings.TrimRight(string(body), "\n"), "\n") {
		if line == "" {
			continue
		}
		var n Note
		require.NoError(t, json.Unmarshal([]byte(line), &n))
		notes = append(notes, n)
	}
	return notes
}

// §3.6.1: a note carries an id of the form `<ISSUE-KEY>#n<n>`, a timestamp, the
// pull request it came from, and the source. The file is read back rather than
// the return value trusted, because it is the file every later round loads.
func TestAppendRecordsTheFieldsTheSpecNames(t *testing.T) {
	l := storeRoot(t)
	at := time.Date(2026, 8, 30, 9, 15, 0, 0, time.FixedZone("UTC+3", 3*60*60))

	first, err := Append(l, "CR-1", "the deadline moved to Friday", SourceChat, 42, at)
	require.NoError(t, err)
	assert.Equal(t, "CR-1#n1", first.ID)
	assert.Equal(t, "the deadline moved to Friday", first.Text)
	assert.Equal(t, SourceChat, first.Source)
	assert.Equal(t, 42, first.PR)
	assert.True(t, at.Equal(first.RecordedAt), "the timestamp is the instant it was given")
	assert.Equal(t, time.UTC, first.RecordedAt.Location(),
		"stored in UTC, so two machines order two notes the same way")

	second, err := Append(l, "CR-1", "the API team owns the retry", SourceMeeting, 42, at)
	require.NoError(t, err)
	assert.Equal(t, "CR-1#n2", second.ID, "§3.6.1 numbers over the notes already stored")

	assert.Equal(t, []Note{first, second}, stored(t, l, "CR-1"),
		"the store holds both notes, in the order they were appended")
}

// Nothing that could not become a §3.6.1 note reaches the store, and a refusal
// leaves the file exactly as it was — a run that fails half way through would
// otherwise spend an id on a note nobody recorded.
func TestAppendRefusesWhatCannotBecomeANote(t *testing.T) {
	l := storeRoot(t)
	at := time.Now()
	_, err := Append(l, "CR-1", "recorded", SourceChat, 42, at)
	require.NoError(t, err)

	for name, attempt := range map[string]func() (Note, error){
		"an unlisted source": func() (Note, error) {
			return Append(l, "CR-1", "hearsay", Source("gossip"), 42, at)
		},
		"an absent source": func() (Note, error) { return Append(l, "CR-1", "hearsay", "", 42, at) },
		"an empty note":    func() (Note, error) { return Append(l, "CR-1", "", SourceChat, 42, at) },
		"a blank note":     func() (Note, error) { return Append(l, "CR-1", " \t\n", SourceChat, 42, at) },
		"no pull request":  func() (Note, error) { return Append(l, "CR-1", "hearsay", SourceChat, 0, at) },
		"a negative pull request": func() (Note, error) {
			return Append(l, "CR-1", "hearsay", SourceChat, -1, at)
		},
		"a key that is a path": func() (Note, error) {
			return Append(l, "../escaped", "hearsay", SourceChat, 42, at)
		},
	} {
		t.Run(name, func(t *testing.T) {
			recorded, err := attempt()
			require.Error(t, err)
			assert.Zero(t, recorded)
		})
	}

	notes := stored(t, l, "CR-1")
	require.Len(t, notes, 1, "a refusal must add nothing")
	assert.Equal(t, "CR-1#n1", notes[0].ID, "and must spend no id")
	assert.NoFileExists(t, filepath.Join(filepath.Dir(l.Root()), "escaped.ndjson"))
}

// §3.6.4 loads a key's notes on every subsequent round and every subsequent
// pull request that resolves to the same key, and §9.3.5 exempts the store from
// round scoping outright. So the pull request is provenance on the record and
// never a scope on the file: a note recorded from one pull request is in the
// same store as a note recorded from another, and a second issue key is a
// second store that numbers from one.
func TestNotesAccumulateAgainstTheKeyAndNotThePullRequest(t *testing.T) {
	l := storeRoot(t)
	at := time.Now()

	// 1 is the lowest pull request GitHub issues, and it is the value that
	// tells Append's `pr < 1` from `pr <= 1`. A first note from PR 42 would
	// pass under either bound.
	_, err := Append(l, "CR-1", "from the first PR", SourceChat, 1, at)
	require.NoError(t, err)
	_, err = Append(l, "CR-1", "from the second PR", SourceThread, 8, at)
	require.NoError(t, err)
	_, err = Append(l, "CR-2", "another issue entirely", SourceJira, 8, at)
	require.NoError(t, err)

	shared := stored(t, l, "CR-1")
	require.Len(t, shared, 2, "one key, one store, whatever pull request each note came from")
	assert.Equal(t, []string{"CR-1#n1", "CR-1#n2"}, []string{shared[0].ID, shared[1].ID})
	assert.Equal(t, []int{1, 8}, []int{shared[0].PR, shared[1].PR},
		"the pull request is recorded on the note, not folded into the file it lives in")

	other := stored(t, l, "CR-2")
	require.Len(t, other, 1)
	assert.Equal(t, "CR-2#n1", other[0].ID, "another key numbers from one")
}

// §3.6.6: a note is unverified hearsay and stays revocable. What retracting one
// does to the store is the whole question this task had to settle, and it is
// settled by marking the note rather than deleting its line.
//
// Both halves are asserted off disk. The note is still there, still carrying
// its text and its number, in the position it was written in — that is what
// keeps §3.3.2's claims and §8.1.6's provenance able to name it, and what stops
// §3.6.1's counter from handing its number to the next note. And it now carries
// the retraction, in UTC, which is what tells a later reader that a citation of
// it may no longer be built on.
func TestRetractMarksTheNoteAndKeepsItInTheStore(t *testing.T) {
	l := storeRoot(t)
	at := time.Date(2026, 8, 30, 9, 15, 0, 0, time.UTC)

	_, err := Append(l, "CR-1", "the deadline moved to Friday", SourceChat, 42, at)
	require.NoError(t, err)
	_, err = Append(l, "CR-1", "the retry limit is three", SourceMeeting, 42, at)
	require.NoError(t, err)

	pulled := at.Add(time.Hour)
	back, err := Retract(l, "CR-1#n1", pulled.In(time.FixedZone("UTC+3", 3*60*60)))
	require.NoError(t, err)
	assert.Equal(t, "CR-1#n1", back.ID)

	held := stored(t, l, "CR-1")
	require.Len(t, held, 2, "§3.6.6 revokes a note; it does not erase one")
	assert.Equal(t, "CR-1#n1", held[0].ID, "the id is spent for the life of the issue")
	assert.Equal(t, "the deadline moved to Friday", held[0].Text,
		"the text is kept, so the records that rested on it can still be recognised")
	assert.Equal(t, SourceChat, held[0].Source)
	require.NotNil(t, held[0].RetractedAt)
	assert.Equal(t, pulled, held[0].RetractedAt.UTC())
	assert.Equal(t, time.UTC, held[0].RetractedAt.Location(),
		"stamped in UTC, so two machines order one store the same way")

	assert.Nil(t, held[1].RetractedAt, "retracting one note retracts one note")
	assert.Equal(t, "the retry limit is three", held[1].Text)

	assert.Equal(t, StandingRetracted, StandingOf(held, "CR-1#n1"))
	assert.Equal(t, StandingStands, StandingOf(held, "CR-1#n2"))
	assert.Equal(t, "CR-1#n3", NextID("CR-1", held),
		"§3.6.1's counter never hands a retracted note's number to another note")
}

// Retracting a note that is already retracted is the state the caller asked
// for, so it succeeds — and the first retraction's timestamp is the one that
// survives. The retraction is a decision somebody made at a moment, and a
// second run reporting the moment it was repeated would lose when it was made.
func TestRetractingTwiceKeepsTheFirstDecision(t *testing.T) {
	l := storeRoot(t)
	at := time.Date(2026, 8, 30, 9, 15, 0, 0, time.UTC)

	_, err := Append(l, "CR-1", "the deadline moved to Friday", SourceChat, 42, at)
	require.NoError(t, err)

	first := at.Add(time.Hour)
	_, err = Retract(l, "CR-1#n1", first)
	require.NoError(t, err)

	again, err := Retract(l, "CR-1#n1", first.Add(24*time.Hour))
	require.NoError(t, err, "the store is already in the state that was asked for")
	require.NotNil(t, again.RetractedAt)
	assert.Equal(t, first, again.RetractedAt.UTC())

	held := stored(t, l, "CR-1")
	require.Len(t, held, 1)
	require.NotNil(t, held[0].RetractedAt)
	assert.Equal(t, first, held[0].RetractedAt.UTC(), "the second run did not restamp the file")
}
