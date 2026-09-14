package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// §8.4.4's match holds a review's commit against the commit_id posted.json
// stores, which is the commit the round's call pinned its review to, and not
// against the round's head read afresh.
//
// The stored commit is deliberately not the round's head here, which no send
// writes: it is the one arrangement in which the two readings disagree. The
// review at the stored commit is adopted, and the review at the round's head is
// passed over with the stored commit named in its reason. A posted.json written
// before cr sent a commit_id stores none, and falls back to the round's head,
// which TestReconcileAdoptsTheRoundsOwnReviewPastOnesThatAreNot covers.
func TestReconcileHoldsAReviewAgainstTheCommitPostedJSONStores(t *testing.T) {
	layout, hash := anUnresolvedPostingAt(t, roundOneCommit)
	body := reviewBodyCarrying(hash)
	reviewListingShim(t,
		[]listedReview{{id: adoptedReviewID, review: aPostedReview()}},
		reviewNode(roundOneReviewID, roundOneReviewURL, draftHead, body),
		reviewNode(adoptedReviewID, adoptedReviewURL, roundOneCommit, body))

	printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
	require.NoError(t, err)

	report := reconcileReport(t, printed)
	assert.Equal(t, adoptedReviewURL, report.Adopted, "the review at the commit the call sent is the round's")
	assert.Equal(t, []string{"f1", "f2"}, report.Records)
	assert.Equal(t, []notAdopted{{
		Review: roundOneReviewURL,
		Reason: "its commit " + draftHead + " is not " + roundOneCommit + ", the commit round 2's review was sent for",
	}}, report.NotAdopted)
	assert.False(t, report.Unresolved)
	for id, record := range roundRecordsByID(t, layout) {
		assert.Equal(t, finding.StatePosted, record.State, id)
	}
}
