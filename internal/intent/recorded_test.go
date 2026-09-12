package intent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A round read back out of §2.3 answers §4.5.3 the way the run that opened it
// did, and says no more than meta.json knows.
//
// Both halves are asserted because both are read. §10.1.3 has `cr status`
// report the axes of the round that was opened, so a key meta.json recorded
// must leave the intent axis available — otherwise the report would claim a
// lens did not look when it looked — and a round that recorded none must leave
// it unavailable with the reason §4.5.4 requires, naming the pattern that was
// searched for.
//
// The origin is asserted because it is the whole of what distinguishes this
// from a live resolution. §3.2 orders four sources and meta.json records none
// of them, so a value claiming one would be cr inventing where the key came
// from; KeyRecorded is no source of §3.2's and nothing that branches on the
// four can mistake it for one.
func TestARecordedRoundAnswersTheIntentAxisWithoutReresolving(t *testing.T) {
	t.Run("a key the round recorded", func(t *testing.T) {
		recorded := Recorded("CR-123", specDefaultPattern)

		assert.Equal(t, Key{Value: "CR-123", Origin: KeyRecorded}, recorded.Key)
		assert.Empty(t, recorded.Text, "meta.json records the key and never the issue text")

		unavailable, marked := recorded.Unavailability()
		assert.False(t, marked, "§4.5.3 applies when no key resolved, and one did")
		assert.Equal(t, Unavailable{}, unavailable)
	})

	t.Run("a round that recorded none", func(t *testing.T) {
		recorded := Recorded("", specDefaultPattern)

		assert.Equal(t, Key{}, recorded.Key, "§3.2's absent key is the zero one")

		unavailable, marked := recorded.Unavailability()
		require.True(t, marked, "§4.5.3 marks the intent axis unavailable when no key resolved")
		assert.Contains(t, unavailable.Reason, specDefaultPattern,
			"§4.5.4's reason names the pattern that was searched for, which is why it is carried")
	})
}
