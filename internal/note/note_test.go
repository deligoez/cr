package note

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// retracted is one note §3.6.6 has retracted, which is a note the store still
// holds rather than a line it no longer has.
func retracted(id string) Note {
	at := time.Date(2026, 8, 30, 11, 0, 0, 0, time.UTC)
	return Note{ID: id, RetractedAt: &at}
}

// §3.6.3 closes the set of sources, so the five it names are admitted and
// nothing else is — an absent flag included, which is the round-11 finding
// unspecified-flag-requiredness: §3.6.1 shows `--source` in its synopsis and
// never says it is required, and a note carrying no provenance cannot make the
// §8.1.6 disclosure that rests on it.
func TestTheSourcesAreTheClosedSetTheSpecNames(t *testing.T) {
	assert.Equal(t,
		[]Source{SourceChat, SourceJira, SourceThread, SourceMeeting, SourceOther},
		Sources(),
		"§3.6.3 names five sources, in this order",
	)

	for _, source := range Sources() {
		parsed, err := ParseSource(string(source))
		require.NoError(t, err)
		assert.Equal(t, source, parsed)
	}

	// The absent flag and the unlisted value fail through one path, so
	// neither can be answered differently from the other.
	for _, value := range []string{"", " ", "gossip", "Chat", "chat ", "email"} {
		var invalid *InvalidSourceError
		_, err := ParseSource(value)
		require.ErrorAs(t, err, &invalid, "%q", value)
		assert.Equal(t, value, invalid.Value)
		assert.Contains(t, err.Error(), "§3.6.3")
		assert.Contains(t, err.Error(), "meeting", "the message names what is admitted")
	}

	assert.Contains(t, (&InvalidSourceError{}).Error(), "--source is required",
		"an absent flag is reported as absent rather than as an unrecognised empty value")

	widened := Sources()
	widened[0] = "gossip"
	assert.Equal(t, SourceChat, Sources()[0], "the set is a copy, so a caller cannot widen it")
}

// §3.6.1 numbers a note `<ISSUE-KEY>#n<n>`, and the number is a count over what
// the store already holds. An id cr did not write is no evidence about what is
// taken, so every spelling but the canonical one contributes nothing rather
// than being read as some number near it — `#n0` included, because notes are
// numbered from one and a zero read as a number would still allocate `#n1`
// while a `-1` would allocate `#n0`.
func TestNextIDCountsOverTheNotesTheStoreHolds(t *testing.T) {
	assert.Equal(t, "CR-1#n1", NextID("CR-1", nil))
	assert.Equal(t, "CR-1#n1", NextID("CR-1", []Note{}))
	assert.Equal(t, "CR-1#n2", NextID("CR-1", []Note{{ID: "CR-1#n1"}}),
		"the first id is spent like any other")
	assert.Equal(t, "CR-1#n3", NextID("CR-1", []Note{{ID: "CR-1#n1"}, {ID: "CR-1#n2"}}))
	assert.Equal(t, "CR-1#n8", NextID("CR-1", []Note{{ID: "CR-1#n7"}, {ID: "CR-1#n2"}}),
		"the highest number is what is spent, not the last one")

	for _, id := range []string{
		"", "CR-1#n0", "CR-1#n-1", "CR-1#n+1", "CR-1#n01", "CR-1#nx", "CR-1#n",
		"CR-1#1", "CR-1", "n1", "OTHER-9#n9", "XCR-1#n9", "cr-1#n9", "CR-1#n1.0",
	} {
		assert.Equal(t, "CR-1#n1", NextID("CR-1", []Note{{ID: id}}), "%q", id)
	}

	assert.Equal(t, "CR-1#n10", NextID("CR-1", []Note{{ID: "CR-1#n9"}, {ID: "CR-1#n0"}}))
	assert.Equal(t, "PROJ-42#n2", NextID("PROJ-42", []Note{{ID: "PROJ-42#n1"}}),
		"the key is part of the id, so another key's ids are another store's")
}

// §3.6.6's retraction is what a citation of a note may still do, and the store
// is the only thing that can answer it. Three standings and not two: an id no
// note bears has exactly as little provenance behind it as a retracted one, so
// §8.1.6 has no region to emit for either and §6.3's register is bought by
// neither. The dangling case is kept separate all the same, because it is the
// one `cr status` and the round summary have something different to say about.
//
// Stands() is asserted beside the standing rather than instead of it: it is the
// single predicate every consumer reads, and a standing that named itself
// correctly while answering the predicate wrongly would let an assertion out.
func TestAStandingIsWhatACitationOfANoteMayStillDo(t *testing.T) {
	held := []Note{{ID: "CR-1#n1"}, retracted("CR-1#n2")}

	assert.Equal(t, StandingStands, StandingOf(held, "CR-1#n1"))
	assert.Equal(t, StandingRetracted, StandingOf(held, "CR-1#n2"))
	assert.Equal(t, StandingDangling, StandingOf(held, "CR-1#n3"),
		"an id the store bears no note for")
	assert.Equal(t, StandingDangling, StandingOf(held, "OTHER-9#n1"),
		"another key's id is another store's, and dangles in this one")
	assert.Equal(t, StandingDangling, StandingOf(nil, "CR-1#n1"),
		"a store nobody has recorded against holds no citation up")

	assert.True(t, StandingStands.Stands())
	assert.False(t, StandingRetracted.Stands(),
		"§3.6.6: a record resting on a retracted note may not assert on it")
	assert.False(t, StandingDangling.Stands(),
		"§8.1.6 has no provenance to disclose for a dangling note id either")

	stands, pulled := Note{ID: "CR-1#n1"}, retracted("CR-1#n2")
	assert.Equal(t, StandingStands, stands.Standing())
	assert.Equal(t, StandingRetracted, pulled.Standing())
	assert.False(t, stands.Retracted())
	assert.True(t, pulled.Retracted())
}

// §3.6.1 forms an id as `<ISSUE-KEY>#n<n>`, so `cr note --remove <note-id>`
// needs no issue key of its own — which is why §11's row for it takes none.
// Only the canonical spelling splits, for the reason parseID accepts only the
// canonical spelling: an id cr did not write names no store cr wrote.
func TestSplitIDReadsTheStoreOutOfTheNoteID(t *testing.T) {
	for id, key := range map[string]string{
		"CR-1#n1":       "CR-1",
		"CR-1#n42":      "CR-1",
		"PROJ-42#n7":    "PROJ-42",
		"CR-1#n2#n3":    "CR-1#n2",
		"A#nB#n1":       "A#nB",
		"has space#n1":  "has space",
		"CR-1#n1234567": "CR-1",
	} {
		split, ok := SplitID(id)
		require.True(t, ok, "%q", id)
		assert.Equal(t, key, split, "%q", id)
	}

	for _, id := range []string{
		"", "CR-1", "CR-1#n", "CR-1#n0", "CR-1#n-1", "CR-1#n+1", "CR-1#n01",
		"CR-1#nx", "CR-1#1", "#n1", "#n", "n1", "CR-1#n1.0", "CR-1#n 1",
	} {
		split, ok := SplitID(id)
		assert.False(t, ok, "%q", id)
		assert.Empty(t, split, "%q", id)
	}
}
