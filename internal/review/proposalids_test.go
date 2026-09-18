package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/proposal"
	"github.com/deligoez/cr/internal/state"
)

// A block whose last id is the only free one still offers it, and a block whose
// last id is held offers nothing.
//
// The boundary is what a prompt tells a role. One free id reported as none
// would have the role propose nothing while an id was there, and no refusal
// anywhere would say so — the prompt is the only place that number appears.
func TestABlockWithOneFreeIDStillOffersIt(t *testing.T) {
	for name, tc := range map[string]struct {
		block       IDs
		first, last string
	}{
		"the last id free":   {IDs{First: 200, Last: 200}, "x200", "x200"},
		"the last id held":   {IDs{First: 201, Last: 200}, "", ""},
		"the whole block":    {IDs{First: 101, Last: 200}, "x101", "x200"},
		"one id past a held": {IDs{First: 102, Last: 200}, "x102", "x200"},
	} {
		t.Run(name, func(t *testing.T) {
			first, last := tc.block.spelledAs(proposal.IDOf)
			assert.Equal(t, tc.first, first)
			assert.Equal(t, tc.last, last)
		})
	}
}

// A stored id sitting exactly on a block's first id moves the block past it.
//
// The boundary is the one that hands an id out twice. A comparison that read
// the block's own first id as outside it would leave First where it was, and
// the prompt would offer a role an id a stored proposal already holds — which
// `cr proposals record` then refuses, telling the role its prompt was wrong.
func TestAStoredIDOnTheBlocksFirstMovesItPast(t *testing.T) {
	for name, tc := range map[string]struct {
		held  []heldID
		first int
	}{
		"nothing held":         {nil, 101},
		"the block's first id": {[]heldID{{round: 1, n: 101}}, 102},
		"the block's last id":  {[]heldID{{round: 1, n: 200}}, 201},
		"one before the block": {[]heldID{{round: 1, n: 100}}, 101},
		"one past the block":   {[]heldID{{round: 1, n: 201}}, 101},
		"the first two ids": {
			[]heldID{{round: 1, n: 101}, {round: 1, n: 102}}, 103,
		},
	} {
		t.Run(name, func(t *testing.T) {
			block := blockFor(tc.held, 0, 1)
			assert.Equal(t, 200, block.Last, "a block is a hundred ids wherever it starts")
			assert.Equal(t, tc.first, block.First)
			assert.True(t, block.Holds(block.Start()), "a block holds its own first id")
			assert.True(t, block.Holds(block.Last), "and its own last")
			assert.False(t, block.Holds(block.Start()-1))
			assert.False(t, block.Holds(block.Last+1))
		})
	}
}

// Every prompt of a round is given a proposal block no other prompt is given,
// across both the roles and the units of the grid (§4.6.2).
//
// The assertion is disjointness rather than a table of expected ids, because
// what §4.6.2 promises is that no two prompts share one — a grid whose
// arithmetic moved would satisfy a table written from the same arithmetic and
// still hand two prompts the same id.
func TestEveryPromptsProposalBlockIsItsOwn(t *testing.T) {
	r := handRound()
	seen := make(map[int]string, len(r.Places)*len(r.Units))
	for _, roleID := range r.Places {
		for at := range r.Units {
			block := r.proposalIDs(roleID, at)
			require.Equal(t, idBlock, block.Last-block.Start()+1)
			for n := block.Start(); n <= block.Last; n++ {
				where := roleID + " on " + r.Units[at].ID
				// Every id a prompt offers must be one the
				// recorder accepts. A block that ran below 1
				// would have the prompt name ids `cr proposals
				// record` refuses as misspelled, and the role
				// would be told its own prompt was wrong.
				require.Truef(t, proposal.ValidID(proposal.IDOf(n)),
					"%s offers %s, which §5.7 does not spell", where, proposal.IDOf(n))
				if other, taken := seen[n]; taken {
					require.Failf(t, "shared id",
						"%s is given to %s and to %s", proposal.IDOf(n), other, where)
				}
				seen[n] = where
			}
		}
	}
	assert.Len(t, seen, len(r.Places)*len(r.Units)*idBlock)
}

// A proposal an earlier round stored pushes this round's blocks past it, and
// this round's own proposals do not move them.
//
// The asymmetry is §4.6.2's, and it is the same one record ids follow: an id a
// later round hands out again would name two proposals, while a block that
// moved the moment `cr proposals record` stored the round's first file would
// give a later emission ids the earlier one already handed out.
func TestAnEarlierRoundsProposalsPushTheBlocksPast(t *testing.T) {
	r := handRound()
	first := r.proposalIDs(r.Places[0], 0)

	r.HeldProposals = []proposal.Proposal{
		{Stamp: state.Stamp{Round: r.Round}, ID: proposal.IDOf(first.Start() + 5)},
	}
	assert.Equal(t, first.Start(), r.proposalIDs(r.Places[0], 0).Start(),
		"this round's own proposals leave the block where it is")

	r.HeldProposals = []proposal.Proposal{
		{Stamp: state.Stamp{Round: r.Round - 1}, ID: proposal.IDOf(40)},
	}
	assert.Equal(t, first.Start()+40, r.proposalIDs(r.Places[0], 0).Start(),
		"an earlier round's highest proposal id is what every block starts past")
}
