package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/state"
)

// earlierReviewID is the node id of the review the earlier round's post created.
const earlierReviewID = "PRR_round1"

// laterHead is the head the next round of draftedHome's pull request stands on.
const laterHead = "9a8b7c6d5e4f30211203f4e5d6c7b8a9f0e1d2c3"

// nextRound opens the round after draftedHome's on laterHead, holding records,
// the way `cr brief` leaves a round opened past one that was posted: meta.json
// names the new round and its records join findings.ndjson beside the earlier
// round's.
func nextRound(t *testing.T, layout state.Layout, records ...*finding.Finding) {
	t.Helper()
	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum,
		Round: draftRound + 1, Head: laterHead,
	}))
	require.NoError(t, state.AppendStamped(
		held, state.FileFindings, state.Stamp{Head: laterHead, Round: draftRound + 1}, records,
	))
	require.NoError(t, held.Unlock())
}

// threadsOfRound are the record ids of one round's records keyed to the thread
// id each carries in findings.ndjson, and the thread ids that round's
// posted.json keys by record.
func threadsOfRound(t *testing.T, layout state.Layout, round int) (stored, posted map[string]string) {
	t.Helper()
	records, err := state.ReadStamped[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings, round)
	require.NoError(t, err)
	stored = make(map[string]string, len(records))
	for i := range records {
		assert.Equal(t, finding.StatePosted, records[i].State, records[i].ID)
		stored[records[i].ID] = records[i].ThreadID
	}
	body, err := os.ReadFile(layout.RoundFile(draftOwner, draftRepo, draftPRNum, round, state.FilePosted))
	require.NoError(t, err)
	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &document))
	require.NoError(t, json.Unmarshal(document[postedThreads], &posted))
	return stored, posted
}

// §8.3.3 and §6.1's `thread_id` across two rounds that say the same thing at
// the same place: round 2 posts f1, and round 3 posts f2 with f1's body on
// f1's lines. The pull request then holds two threads a body and position match
// cannot tell apart, and the thread query lists round 2's first.
//
// Round 3's record takes the thread the review its own send created opened —
// the review `cr post --confirm`'s call answered with, or the one
// `cr post --reconcile` adopted — and round 2's record keeps its own. Reading
// every thread on the pull request would hand f2 the thread the author already
// answered a round ago.
func TestAThreadAnEarlierRoundOpenedIsNotClaimedForThisRound(t *testing.T) {
	for _, send := range []struct {
		name  string
		reach func(t *testing.T, later *post.Review, listed ...listedReview)
		// review is the review whose threads are the later round's.
		review string
	}{
		{
			name:   "confirm",
			review: createdReviewID,
			reach: func(t *testing.T, _ *post.Review, listed ...listedReview) {
				ghShimmingAs(t, createdReviewID, listed...)
				_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
				require.NoError(t, err)
			},
		},
		{
			name:   "reconcile",
			review: adoptedReviewID,
			reach: func(t *testing.T, later *post.Review, listed ...listedReview) {
				reconcilingShimListing(t, later, listed...)
				_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
				require.Error(t, err, "§8.4.4: the send meets a gh that dies without answering")
				_, err = runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
				require.NoError(t, err)
			},
		},
	} {
		t.Run(send.name, func(t *testing.T) {
			earlier, later := aCitedRecord("f1"), aCitedRecord("f2")
			earlier.Summary = "The error Decode returns is dropped."
			later.Summary = earlier.Summary
			layout := draftedHome(t, earlier)
			redraft(t)
			first := builtPayload(t)
			require.Len(t, first.Comments, 1)
			ghShimmingAs(t, earlierReviewID, listedReview{id: earlierReviewID, review: first})
			_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
			require.NoError(t, err)

			nextRound(t, layout, later)
			redraft(t)
			second := builtPayload(t)
			require.Len(t, second.Comments, 1)
			require.Equal(t, first.Comments[0], second.Comments[0],
				"the later round's comment has the earlier one's body at the earlier one's position")

			send.reach(t, second,
				listedReview{id: earlierReviewID, review: first},
				listedReview{id: send.review, review: second})

			stored, posted := threadsOfRound(t, layout, draftRound+1)
			want := map[string]string{"f2": threadIDFor(1)}
			assert.Equal(t, want, stored,
				"§6.1: the later round's record carries the thread its own review opened")
			assert.Equal(t, want, posted,
				"§8.3.3: the later round's posted.json names the thread its own review opened")

			stored, posted = threadsOfRound(t, layout, draftRound)
			assert.Equal(t, map[string]string{"f1": threadIDFor(0)}, stored,
				"the earlier round's record keeps the thread it was given")
			assert.Equal(t, map[string]string{"f1": threadIDFor(0)}, posted)
		})
	}
}

// §8.3.3's read-back when the review cannot be named: a created call whose
// answer carries no `node_id` names no review, so no thread is claimed for the
// round — not even one whose opening comment GitHub names no review for, which
// would match an empty id if the two were compared. The records are still
// posted, and they carry no thread id rather than a guessed one.
func TestAReviewTheCallDoesNotNameClaimsNoThread(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"))
	redraft(t)
	payload := builtPayload(t)
	ghShimmingAs(t, "", listedReview{review: payload})

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)

	stored, posted := threadsOfRound(t, layout, draftRound)
	assert.Equal(t, map[string]string{"f1": ""}, stored,
		"§6.1: a thread cr could not tie to the review is not recorded")
	assert.Empty(t, posted, "§8.3.3: posted.json names no thread it could not tie to the review")
}
