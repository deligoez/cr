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
	"answered",
	"addressed",
	"withdrawn",
	"stale",
}

// specTerminal is §9.1.2's list, transcribed in the order that item writes it.
//
// `posted` is absent from v0.5 onward and its absence is the release: §9.1.2
// calls a posted record open, and §9.1.3 has §7.3's statistics and §10.2's
// completeness count it so. A transcription that kept it would pass while the
// loop stayed one round long.
var specTerminal = []string{
	"answered", "addressed", "withdrawn", "discarded", "duplicate", "suppressed", "stale",
}

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
// than the presence of each. §9.1's table, its transition table and §9.1.2's
// terminal list are one vocabulary read three ways, so an eleventh state is not
// an addition — it is a release half-implemented, with the transition table
// silent about how a record reaches it and §9.1.2 silent about whether it is
// open. A missing one is worse: nothing would name what a record
// suppressed by §3.5.4 or abandoned by §9.3.4 has become.
func TestTheTenStatesAreTheOnesTheSpecWrites(t *testing.T) {
	assert.Equal(t, specStates, names(States()))

	for _, name := range specStates {
		parsed, err := ParseState(name)
		require.NoError(t, err)
		assert.Equal(t, name, parsed.String())
		assert.True(t, parsed.Valid())
	}

	// Three plausible names the re-review half does not use. They are
	// absent by being unrepresentable: State's only field is unexported, so
	// nothing outside state.go can build one, and this is the door a name
	// has to come through.
	//
	// v0.5 added the states these were once standing in for, and took none
	// of these three. Verifying writes `answered` or `addressed` depending
	// on what settled the record, so `verified` would say less than either;
	// resolving a thread is a write to GitHub that §9.6.1 permits only on a
	// record already settled, so it moves nothing and names nothing here;
	// and §1.3.3 keeps cr out of approving at all, so `accepted` is a
	// verdict it may never reach.
	for _, absent := range []string{"verified", "resolved", "accepted"} {
		_, err := ParseState(absent)
		var unknown *UnknownStateError
		require.ErrorAs(t, err, &unknown)
		assert.Equal(t, absent, unknown.Value)
		assert.Contains(t, err.Error(), "verifying writes answered or addressed")
	}

	// The set is closed to its callers too, or the copy States hands back
	// would be the vocabulary itself.
	widened := States()
	widened[0] = StatePosted
	assert.Equal(t, specStates, names(States()))
}

// §1.1 defines Open as any record in a non-terminal state per §9.1, which makes
// the open set a derivation and not a list. Both halves are asserted against
// §9.1's whole table so that they stay a partition of it: a state that fell out
// of both halves — or into both — would make §10.2's completeness a wrong
// verdict about whether a round is finished, in either direction.
//
// `posted` joined the open set in v0.5 and the assertion below is written out
// rather than derived, because that membership is the release. §10.2.4 reads
// completeness over unposted work and is unaffected; §9.1.3 is what makes the
// difference visible, by having §7.3 count an `answered` record settled and a
// `posted` one still open.
func TestOpenIsEveryStateThatIsNotTerminal(t *testing.T) {
	open, closed := make([]string, 0), make([]string, 0)
	for _, s := range States() {
		require.NotEqual(t, s.Open(), s.Terminal(), "%s is neither open nor terminal, or both", s)
		if s.Terminal() {
			closed = append(closed, s.String())
			continue
		}
		open = append(open, s.String())
	}
	assert.ElementsMatch(t, specTerminal, closed)
	assert.Equal(t, []string{"draft", "queued", "posted"}, open,
		"§9.1.2 from v0.5: a posted concern nobody has settled is open")
	assert.Equal(t, open, names(OpenStates()))

	// A record that has been through no §9.1 transition is in no state at
	// all. Reading it as open would make every unstamped record hold a
	// round back; reading it as terminal would let one out unposted.
	var none State
	assert.False(t, none.Valid())
	assert.False(t, none.Open())
	assert.False(t, none.Terminal())
}

// A state exists to be written to `findings.ndjson` and read back, and the
// vocabulary is only closed if the file cannot widen it. §6.1.4 has cr write
// the field and rejects a record arriving with it, so a line naming an eighth
// state was not produced by this cr, and carrying it forward would let §9.1's
// transition table and §10.2.4's completeness check reason about a state
// neither of them has a rule for.
func TestAStateOutsideTheTableNeverReachesARecord(t *testing.T) {
	assert.Contains(t, string(mustMarshal(t, &Finding{ID: "f1", State: StateQueued})), `"state":"queued"`)
	// A record with no state carries no key, rather than one holding "".
	assert.NotContains(t, string(mustMarshal(t, &Finding{ID: "f1"})), "state")

	var stored Finding
	err := json.Unmarshal([]byte(`{"id":"f1","state":"verified"}`), &stored)
	var unknown *UnknownStateError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, "verified", unknown.Value)

	// null is the one value this refuses to answer. It is a key the agent
	// wrote, so §6.1.4's fence owns the rejection — which reads the wire
	// keys rather than the decoded record — and failing here would report
	// it as malformed JSON instead.
	// TestARecordArrivingWithAComputedFieldIsRejected is the other half.
	var record Finding
	require.NoError(t, json.Unmarshal([]byte(`{"id":"f1","state":null}`), &record))
	assert.False(t, record.State.Valid())
}
