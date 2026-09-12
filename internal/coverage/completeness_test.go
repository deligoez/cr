package coverage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// completeRound is a round on which all four §10.2 conditions hold: the head
// has not moved, both units hold a complete row for every active role, every
// claim is settled, and every record has left §9.1.2's open set.
//
// Each test below breaks exactly one of the four, so what a failing test names
// is the condition it broke rather than the fixture.
func completeRound() Conditions {
	return Conditions{
		HeadMoved: false,
		Rows:      Rows{Units: 2, Complete: 2, Gaps: 0, Oversized: 1, Roles: 3},
		Unsettled: []string{},
		Records: []*finding.Finding{
			{ID: "f1", State: finding.StatePosted},
			{ID: "f2", State: finding.StateDuplicate},
		},
	}
}

// All four §10.2 conditions holding is a complete round, and a complete round
// carries no reason: §10.2 asks for one only when the verdict is false, and
// there is none beyond the four conditions all holding.
func TestARoundMeetingAllFourConditionsIsComplete(t *testing.T) {
	held := completeRound()

	verdict := Complete(&held)

	assert.True(t, verdict.Complete)
	assert.Empty(t, verdict.Reasons)
	assert.Empty(t, verdict.Reason())
}

// Each of §10.2's four conditions blocks completeness on its own.
//
// They are broken one at a time rather than together, because a verdict that
// read only the first condition would pass a round failing the fourth and no
// test over a round failing all four would notice. The reason is asserted to
// name the item it came from, which is what makes "the exact reason" checkable
// by a reviewer who has §10.2 open.
func TestEachOfTheFourConditionsBlocksCompletenessOnItsOwn(t *testing.T) {
	for name, broken := range map[string]struct {
		break_ func(*Conditions)
		reason string
	}{
		"§10.2.1 the head moved": {
			break_: func(c *Conditions) { c.HeadMoved = true },
			reason: "§10.2.1: the pull request's head is no longer the head this round " +
				"was recorded against",
		},
		"§10.2.2 a unit has no complete row": {
			break_: func(c *Conditions) {
				c.Rows = Rows{Units: 2, Complete: 1, Gaps: 1, Oversized: 1, Roles: 3}
			},
			reason: "§10.2.2: 1 of 2 unit(s) hold no complete row of cells for all " +
				"3 active role(s) at their current unit hash",
		},
		"§10.2.3 a claim is mapped to nothing": {
			break_: func(c *Conditions) { c.Unsettled = []string{"CR-7#c2", "CR-7#c3"} },
			reason: "§10.2.3: 2 claim(s) are mapped to no unit and not set aside: " +
				"CR-7#c2, CR-7#c3",
		},
		"§10.2.4 a record is still open": {
			break_: func(c *Conditions) {
				c.Records = append(c.Records, &finding.Finding{ID: "f3", State: finding.StateQueued})
			},
			reason: "§10.2.4: 1 record(s) are still in draft or queued: f3",
		},
	} {
		t.Run(name, func(t *testing.T) {
			held := completeRound()
			broken.break_(&held)

			verdict := Complete(&held)

			assert.False(t, verdict.Complete)
			require.Len(t, verdict.Reasons, 1,
				"exactly one condition was broken, so exactly one reason is exact")
			assert.Equal(t, broken.reason, verdict.Reasons[0])
			assert.Equal(t, broken.reason, verdict.Reason())
		})
	}
}

// A claim carrying §4.1.3's unimplemented entry blocks until it is set aside
// per §4.1.8, and stops blocking once it is.
//
// This is the condition §4.1.3 relies on. The entry is not a record, so
// §10.2.4 never reaches it, and §10.2.3 is the only thing that makes an
// unimplemented claim block at all — without it a round could be reported
// complete while a claim the issue asks for is implemented nowhere and the
// reviewer was never told.
func TestAnUnimplementedClaimBlocksUntilItIsMappedOrSetAside(t *testing.T) {
	blocked := completeRound()
	blocked.Unsettled = []string{"CR-7#c2"}
	require.False(t, Complete(&blocked).Complete)

	// Set aside per §4.1.8, which is what takes it out of §10.2.3's
	// blocking set: `cr claims set-aside` stamps the entry, and
	// `cr map record` carries the stamp forward.
	settled := completeRound()

	assert.True(t, Complete(&settled).Complete)
}

// Several broken conditions are all reported, in §10.2's own numbering.
//
// A reviewer shown only the first would fix it, run again, and be told about
// the second — the same report delivered one round at a time, on a loop that
// §1.3.6 says may end at any round.
func TestEveryBrokenConditionIsReportedInTheOrderSection102NumbersThem(t *testing.T) {
	held := completeRound()
	held.HeadMoved = true
	held.Rows = Rows{Units: 2, Complete: 1, Gaps: 1, Oversized: 1, Roles: 3}
	held.Unsettled = []string{"CR-7#c2"}
	held.Records = append(held.Records, &finding.Finding{ID: "f3", State: finding.StateDraft})

	verdict := Complete(&held)

	require.Len(t, verdict.Reasons, 4)
	for at, item := range []string{"§10.2.1", "§10.2.2", "§10.2.3", "§10.2.4"} {
		assert.Contains(t, verdict.Reasons[at], item)
	}
	assert.Contains(t, verdict.Reason(), "; ", "the exact reason is one sentence")
}

// §10.2.4's sentence names the states it actually blocks on, read out of
// §9.1.2's open set rather than spelled here.
//
// A state added to §9.1 as non-terminal would block completeness through
// State.Open and be named by the same call, so the predicate and the sentence
// cannot come apart — which is the failure a hardcoded "draft or queued" would
// hide: a round blocked by a third open state, reported as blocked by two.
func TestTheOpenStatesNamedInTheReasonAreTheOnesThatBlock(t *testing.T) {
	for _, state := range finding.OpenStates() {
		held := completeRound()
		held.Records = append(held.Records, &finding.Finding{ID: "f3", State: state})

		verdict := Complete(&held)

		require.False(t, verdict.Complete, "%s is open, so it blocks §10.2.4", state)
		assert.Contains(t, verdict.Reason(), state.String())
	}
}
