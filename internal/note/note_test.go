package note

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
