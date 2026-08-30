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

