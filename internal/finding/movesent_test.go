package finding

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// sentHome is a pull request's state whose findings.ndjson holds the lines
// given, and the lock on it.
func sentHome(t *testing.T, lines ...string) (state.Layout, *state.Lock) {
	t.Helper()
	layout := state.New(t.TempDir())
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR("acme", "shop", 7))
	held, err := layout.LockPR("acme", "shop", 7)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings, []byte(strings.Join(lines, "\n")+"\n")))
	return layout, held
}

// The states whose comment reached GitHub are `posted` and the three §9.1 moves
// it to, and no state a record passes through before posting.
func TestTheSentStatesArePostedAndItsThreeExits(t *testing.T) {
	assert.Equal(t, []State{StatePosted, StateAnswered, StateAddressed, StateWithdrawn}, SentStates())
	for _, unsent := range UnsentStates() {
		assert.False(t, unsent.Sent(), "%s has not reached GitHub", unsent)
	}
	assert.False(t, StateDiscarded.Sent(), "a discarded record was never posted")
	assert.False(t, StateStale.Sent(), "a stale record was abandoned unposted")
}

// MoveSent changes the one line naming the record, in the round that holds it,
// and leaves the line's round and head and every other line alone.
func TestMoveSentChangesTheStateWhereTheRecordLives(t *testing.T) {
	earlier := `{"id":"f3","state":"posted","head":"aaa","round":1}`
	current := `{"id":"f9","state":"queued","head":"bbb","round":2}`
	layout, held := sentHome(t, earlier, current)

	journal := NewJournal(ActorVerify, "bbb", time.Now())
	require.NoError(t, journal.Move("f3", Existing(StatePosted), StateAnswered))
	require.NoError(t, MoveSent(held, "f3", StatePosted, StateAnswered, journal))
	require.NoError(t, held.Unlock())

	body, err := os.ReadFile(layout.PRFile("acme", "shop", 7, state.FileFindings))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	require.Len(t, lines, 2, "the record was moved, not copied")
	assert.JSONEq(t, `{"id":"f3","state":"answered","head":"aaa","round":1}`, lines[0])
	assert.Equal(t, current, lines[1], "a line the move does not name is kept byte for byte")

	transitions, err := os.ReadFile(layout.PRFile("acme", "shop", 7, state.FileTransitions))
	require.NoError(t, err)
	assert.Contains(t, string(transitions), `"record":"f3"`, "§9.1.1's line is written with the move")
}

// The state the caller's lock-free read saw is checked again under the lock: a
// record another run settled in between is refused as the §9.1 conflict it is,
// and nothing is written.
func TestMoveSentRefusesARecordThatMovedUnderneathIt(t *testing.T) {
	settled := `{"id":"f3","state":"withdrawn","head":"aaa","round":1}`
	layout, held := sentHome(t, settled)

	journal := NewJournal(ActorVerify, "bbb", time.Now())
	require.NoError(t, journal.Move("f3", Existing(StatePosted), StateAddressed))
	err := MoveSent(held, "f3", StatePosted, StateAddressed, journal)
	require.NoError(t, held.Unlock())

	var illegal *IllegalTransitionError
	require.ErrorAs(t, err, &illegal)
	assert.Equal(t, Existing(StateWithdrawn), illegal.From, "the refusal names where the record now stands")
	body, err := os.ReadFile(layout.PRFile("acme", "shop", 7, state.FileFindings))
	require.NoError(t, err)
	assert.Equal(t, settled+"\n", string(body))
	transitions, err := os.ReadFile(layout.PRFile("acme", "shop", 7, state.FileTransitions))
	if !os.IsNotExist(err) {
		require.NoError(t, err)
	}
	assert.NotContains(t, string(transitions), `"record":"f3"`,
		"no journal line is written for a move that did not happen")
}

// A record id that no line holds publishes nothing: an id is one record's for
// the life of the pull request, and a move that found none found the wrong file.
func TestMoveSentRefusesAnIDNoLineHolds(t *testing.T) {
	_, held := sentHome(t, `{"id":"f3","state":"posted","head":"aaa","round":1}`)
	journal := NewJournal(ActorVerify, "bbb", time.Now())
	err := MoveSent(held, "f8", StatePosted, StateAnswered, journal)
	require.NoError(t, held.Unlock())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0 lines for record f8")
}
