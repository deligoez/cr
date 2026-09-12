package mapping

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// entry is one recorded §4.1.3 line, spelled as the three things a set-aside
// can read: the claim, the round it stands in, and whatever stamp it carries.
func entry(claim string, round int, note string) Gap {
	return Gap{Claim: claim, Stamp: state.Stamp{Head: "abc1234", Round: round}, SetAsideNote: note}
}

// rendered is a result as `claim@round/note`, which is the whole of what
// SetAside can be right or wrong about.
func rendered(gaps []*Gap) []string {
	out := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		out = append(out, gap.Claim+"@"+string(rune('0'+gap.Round))+"/"+gap.SetAsideNote)
	}
	return out
}

// §4.1.8 stamps the note on the named claim's entry and returns the whole
// round.
//
// The other entry of the round is the assertion that matters. The caller hands
// this result to state.ReplaceStamped, which keeps nothing it is not given, so
// an implementation returning only the entry it changed would delete the
// neighbour — and §10.2.3 would stop blocking on an unimplemented claim nobody
// judged.
func TestSetAsideStampsOneEntryAndReturnsTheWholeRound(t *testing.T) {
	recorded := []Gap{entry("CR-1#c1", 2, ""), entry("CR-1#c2", 2, "")}

	stamped, err := SetAside(recorded, "CR-1#c1", "CR-1#n3", 2)

	require.NoError(t, err)
	assert.Equal(t, []string{"CR-1#c1@2/CR-1#n3", "CR-1#c2@2/"}, rendered(stamped))
}

// An entry of another round is neither stamped nor returned, per §9.3.5.
//
// Both halves matter and they fail in opposite directions. Stamping it would
// put the reviewer's judgement about this round onto a round that is history;
// returning it would hand state.ReplaceStamped a line belonging to a round it
// is not replacing, which would restamp the history it is meant to leave alone.
func TestSetAsideReadsAndWritesOnlyTheRoundItWasGiven(t *testing.T) {
	recorded := []Gap{
		entry("CR-1#c1", 1, "CR-1#n1"),
		entry("CR-1#c1", 2, ""),
	}

	stamped, err := SetAside(recorded, "CR-1#c1", "CR-1#n3", 2)

	require.NoError(t, err)
	assert.Equal(t, []string{"CR-1#c1@2/CR-1#n3"}, rendered(stamped),
		"§9.3.5: round 1's entry is history, and the write is the current round's")
}

// A claim the round raises no entry for is refused, and nothing is returned to
// write.
//
// The claim exists in an earlier round here, which is the case a lookup keyed
// on the claim alone would get wrong: it would stamp a judgement about this
// round onto a line belonging to one that has closed.
func TestSetAsideRefusesAClaimTheRoundRaisesNoEntryFor(t *testing.T) {
	recorded := []Gap{entry("CR-1#c1", 1, ""), entry("CR-1#c2", 2, "")}

	stamped, err := SetAside(recorded, "CR-1#c1", "CR-1#n3", 2)

	assert.Nil(t, stamped, "a refused set-aside gives the caller nothing to write")
	var missing *NoGapError
	require.ErrorAs(t, err, &missing)
	assert.Equal(t, "CR-1#c1", missing.Claim)
	assert.Equal(t, 2, missing.Round,
		"the refusal names the round searched, because the same claim can raise an entry in another")
	assert.Contains(t, err.Error(), "cr status",
		"§12.4: the refusal names the next actionable step")
}

// A second set-aside replaces the note rather than refusing or appending.
//
// §4.1.8 gives the field no history, and correcting which note a decision cites
// is the agent's to do — cr judging that the first reference was the real one
// would be forming the opinion §4.1.8 reserves.
func TestASecondSetAsideReplacesTheNote(t *testing.T) {
	recorded := []Gap{entry("CR-1#c1", 2, "CR-1#n1")}

	stamped, err := SetAside(recorded, "CR-1#c1", "CR-1#n2", 2)

	require.NoError(t, err)
	assert.Equal(t, []string{"CR-1#c1@2/CR-1#n2"}, rendered(stamped))
}
