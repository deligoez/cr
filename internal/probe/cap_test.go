package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// inRound is a stored probe record, counted only by the round it was written in.
func inRound(round int) Record {
	return Record{Kind: Mutation, Result: resultNoTestFailed, Stamp: state.Stamp{Round: round}}
}

// §5.6.4 caps probe executions per round, so the count is the round's own.
//
// The other rounds' records are the case worth writing: §5.5.1 keeps every
// earlier round's probes on disk and §5.5.3 has already withdrawn their
// standing here, so a count taken over the whole file would let a pull
// request's third round refuse the first probe it asked for.
func TestTheProbeCapCountsThisRoundsProbesAlone(t *testing.T) {
	stored := []Record{inRound(1), inRound(1), inRound(2), inRound(3), inRound(2)}

	assert.Equal(t, 2, RoundCapFor(stored, 1, 10).Count)
	assert.Equal(t, 2, RoundCapFor(stored, 2, 10).Count)
	assert.Equal(t, 1, RoundCapFor(stored, 3, 10).Count)
	assert.Equal(t, 0, RoundCapFor(stored, 4, 10).Count,
		"a round that has run nothing has spent nothing")
	assert.Equal(t, 0, RoundCapFor(nil, 1, 10).Count)
	assert.Equal(t, 10, RoundCapFor(nil, 1, 10).Max,
		"the cap is the resolved setting, passed in rather than read here")
}

// The cap is the last execution that fits: probe.max_per_round 10 lets a round
// run ten probes and refuses the eleventh.
//
// The boundary is asserted from both sides at the default, because an
// off-by-one here is not a rounding error — it either spends a probe the user
// budgeted for or runs one they did not.
func TestTheCapIsTheLastExecutionThatFits(t *testing.T) {
	for name, tc := range map[string]struct {
		ran     int
		max     int
		refuses bool
	}{
		"a round that has run nothing":   {ran: 0, max: 10},
		"a round one short of the cap":   {ran: 9, max: 10},
		"a round that has spent the cap": {ran: 10, max: 10, refuses: true},
		"a round somehow past the cap":   {ran: 11, max: 10, refuses: true},
		"a cap of one admits one probe":  {ran: 0, max: 1},
		"and refuses the second":         {ran: 1, max: 1, refuses: true},
		"a cap of zero admits none":      {ran: 0, max: 0, refuses: true},
	} {
		t.Run(name, func(t *testing.T) {
			capped := RoundCap{Count: tc.ran, Max: tc.max}
			assert.Equal(t, tc.refuses, capped.Reached())
			if !tc.refuses {
				assert.NoError(t, capped.Err())
				return
			}
			var refused *RoundCapReachedError
			require.ErrorAs(t, capped.Err(), &refused)
			assert.Equal(t, capped, refused.Cap)
		})
	}
}

// §5.6.4: the cap being hit is reported, never silently applied.
//
// The report names both numbers and the setting that changes them, because a
// reader told only that cr stopped probing cannot tell a budget from a bug. It
// names the run that was refused too, so the refusal reads as the one
// experiment that did not happen rather than as an unbounded stop.
func TestTheCapReportNamesTheCountTheCapAndTheSetting(t *testing.T) {
	spent := RoundCap{Count: 10, Max: 10}
	assert.Contains(t, spent.Disclosure(), "§5.6.4")
	assert.Contains(t, spent.Disclosure(), "10 of 10 probes run this round")
	assert.Contains(t, spent.Disclosure(), "probe.max_per_round")
	assert.Contains(t, spent.Disclosure(), "the cap is reached")

	within := RoundCap{Count: 3, Max: 10}
	assert.Contains(t, within.Disclosure(), "3 of 10 probes run this round")
	assert.NotContains(t, within.Disclosure(), "the cap is reached",
		"a round with budget left is not told it has none")

	require.Error(t, spent.Err())
	assert.Contains(t, spent.Err().Error(), "this run would be number 11")
	assert.Contains(t, spent.Err().Error(), "nothing was run")
}

// Ran is the cap as it stands once this run is recorded, which is the number a
// run discloses: the reader needs how many probes the round has now spent, not
// how many it had spent before the result they are reading.
func TestTheDisclosedCountIncludesTheRunBeingReported(t *testing.T) {
	before := RoundCap{Count: 9, Max: 10}
	require.NoError(t, before.Err(), "the tenth probe still fits")

	after := before.Ran()
	assert.Equal(t, RoundCap{Count: 10, Max: 10}, after)
	assert.True(t, after.Reached(), "the tenth probe is the one that fills the round")
	assert.Equal(t, RoundCap{Count: 9, Max: 10}, before,
		"the cap the run was admitted under is left alone")
}
