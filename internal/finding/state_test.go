package finding

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// specStates is §9.1's table, transcribed from the spec in its own order.
var specStates = []string{
	"draft",
	"queued",
	"duplicate",
	"suppressed",
	"discarded",
	"posted",
	"stale",
}

// specTerminal is §9.1.2's list, transcribed in the order that item writes it.
var specTerminal = []string{"posted", "discarded", "duplicate", "suppressed", "stale"}

// names renders a set of states as the strings §9.1's table writes, so an
// assertion reads as the table does and a failure names the state.
func names(set []State) []string {
	rendered := make([]string, 0, len(set))
	for _, s := range set {
		rendered = append(rendered, s.String())
	}
	return rendered
}

// mustMarshal encodes a record or fails the test.
func mustMarshal(t *testing.T, record *Finding) []byte {
	t.Helper()
	line, err := json.Marshal(record)
	require.NoError(t, err)
	return line
}

// The vocabulary is the whole of §9, so the exact set is the assertion rather
// than the presence of each. §9's opening paragraph ends a record's life at
// posting and §1.3.6 puts verifying, resolving, and withdrawing in v0.2, so an
// eighth state is not an addition — it is v0.2 half-implemented, with §9.1's
// transition table silent about how a record reaches it and §9.1.2 silent about
// whether it is open. A missing one is worse: nothing would name what a record
// suppressed by §3.5.4 or abandoned by §9.3.4 has become.
func TestTheSevenStatesAreTheOnesTheSpecWrites(t *testing.T) {
	assert.Equal(t, specStates, names(States()))

	for _, name := range specStates {
		parsed, err := ParseState(name)
		require.NoError(t, err)
		assert.Equal(t, name, parsed.String())
		assert.True(t, parsed.Valid())
	}

	// The four states of the re-review half. They are absent by being
	// unrepresentable: State's only field is unexported, so nothing outside
	// state.go can build one, and this is the door a name has to come
	// through.
	for _, v02 := range []string{"verified", "resolved", "accepted", "withdrawn"} {
		_, err := ParseState(v02)
		var unknown *UnknownStateError
		require.ErrorAs(t, err, &unknown)
		assert.Equal(t, v02, unknown.Value)
		assert.Contains(t, err.Error(), "§1.3.6")
	}

	// The set is closed to its callers too, or the copy States hands back
	// would be the vocabulary itself.
	widened := States()
	widened[0] = StatePosted
	assert.Equal(t, specStates, names(States()))
}

