package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// aPostingRound is the round `cr post` would be writing the index from: the
// meta.json the command already holds, which is where recordPostedIndex reads
// the pull request it is appending for.
func aPostingRound(round int, head string) *state.Meta {
	return &state.Meta{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum, Round: round, Head: head,
	}
}

// §9.3.6 over the boundary it exists for: a record posted in round 1 puts an
// entry in `posted-index.ndjson`, and the identical finding raised again in
// round 2 is dropped before anything is written, with the drop counted.
//
// "Identical" is identical in §7.4.1's four fields and in nothing else. The
// round-2 finding carries a fresh id, a different role and a reworded summary,
// because those are exactly what a second round produces — §6.1.1 mints a new
// id every round — and a key carrying any of them would answer "not posted" on
// every round and the index would suppress nothing it was built to suppress.
//
// The drop is asserted to leave the round-2 record out of what the merge hands
// on, rather than to mark it. §9.3.6 drops "exactly as §6.4.4 drops a waived
// finding", and §6.4.4 has the dropped finding never reach `findings.ndjson` at
// all, because §9.1 defines no state for it.
func TestAFindingPostedInRoundOneIsDroppedInRoundTwo(t *testing.T) {
	layout := draftedHome(t)
	posted := aStoredRecord("f1", finding.StatePosted)
	posted.Round, posted.Head = draftRound, draftHead

	require.NoError(t, recordPostedIndex(
		layout, aPostingRound(draftRound, draftHead), []*finding.Finding{posted}, createdReviewID))

	index, err := finding.PostedIndex(layout, draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.Len(t, index, 1, "§9.3.6 holds one entry per record that reached posted")
	assert.Equal(t, "f1", index[0].Record)
	assert.Equal(t, finding.WaiverKeyOf(posted), index[0].WaiverKey,
		"§9.3.6 keys the index exactly as a §7.4.1 waiver is keyed")
	assert.Equal(t, draftRound, index[0].Round)

	again := aStoredRecord("f7", finding.StateDraft)
	again.Round, again.Head = draftRound+1, "9a8b7c6"
	again.Role, again.Summary = "convention", "The returned error is discarded here."
	fresh := aStoredRecord("f8", finding.StateDraft)
	fresh.Round, fresh.Head = draftRound+1, "9a8b7c6"
	fresh.Anchor.ContentHash = "fedcba9876543210"

	kept, drops := finding.DropPosted([]*finding.Finding{again, fresh}, index)

	assert.Equal(t, []*finding.Finding{fresh}, kept,
		"§9.3.6: the finding the author already received never reaches findings.ndjson")
	assert.Equal(t, 1, drops.Dropped, "§6.5.1 writes this count into the round summary")
	assert.Equal(t, []string{"f1"}, drops.Posted,
		"the id named is the comment the author has, not the finding that was dropped")
	assert.Equal(t, "1 finding(s) dropped as already posted, per §9.3.6", drops.Disclosure())
}

// §9.3.5 exempts the posted index from round scoping, and this is the exemption
// doing its work rather than being declared: the entry is read in a later round
// and against a moved head.
//
// The head is the sharper half. §7.4.1's key holds the content hash of the
// anchored lines and not the commit, so a pull request that moved forward
// without touching the code the comment was about still finds the entry —
// which is the case §9.3.6 exists for, since a round is opened by exactly such
// a move.
func TestThePostedIndexIsReadAcrossRoundsAndAcrossHeads(t *testing.T) {
	layout := draftedHome(t)
	posted := aStoredRecord("f1", finding.StatePosted)
	posted.Round, posted.Head = draftRound, draftHead
	require.NoError(t, recordPostedIndex(
		layout, aPostingRound(draftRound, draftHead), []*finding.Finding{posted}, createdReviewID))

	index, err := finding.PostedIndex(layout, draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)

	later := aStoredRecord("f9", finding.StateDraft)
	later.Round, later.Head = draftRound+5, "0f1e2d3"
	entry, found := finding.PostedBefore(index, later)
	assert.True(t, found, "§9.3.5 exempts posted-index.ndjson, so an earlier round's entry still matches")
	assert.Equal(t, draftRound, entry.Round, "the entry still says which round posted it")

	rewritten := aStoredRecord("f9", finding.StateDraft)
	rewritten.Anchor.ContentHash = "0000000000000000"
	_, found = finding.PostedBefore(index, rewritten)
	assert.False(t, found,
		"§7.4.1's key is over the anchored lines, so rewritten code is raised again")
}

// §9.3.6 holds one entry per record, and a second posting of the same code does
// not add a second line.
//
// It is the same narrowness as everything else keyed by §7.4.1: the entry
// covers a defect at unchanged code, so a run that posted it again — which is a
// run in which the drop did not reach it — has nothing new to record.
func TestThePostedIndexHoldsOneEntryPerKey(t *testing.T) {
	layout := draftedHome(t)
	first := aStoredRecord("f1", finding.StatePosted)
	first.Round, first.Head = draftRound, draftHead
	second := aStoredRecord("f4", finding.StatePosted)
	second.Round, second.Head = draftRound+1, "9a8b7c6"

	require.NoError(t, recordPostedIndex(
		layout, aPostingRound(draftRound, draftHead), []*finding.Finding{first}, createdReviewID))
	require.NoError(t, recordPostedIndex(
		layout, aPostingRound(draftRound+1, "9a8b7c6"), []*finding.Finding{second}, createdReviewID))

	index, err := finding.PostedIndex(layout, draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.Len(t, index, 1)
	assert.Equal(t, "f1", index[0].Record, "the entry that was already there is the one kept")
}
