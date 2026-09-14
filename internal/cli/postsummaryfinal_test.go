package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/state"
)

// postFinalisation is the part of a round summary a confirmed `cr post`
// finalises: §10.3's comment count against post.max_comments, its posted count
// and payload hash, and §8.5.4's confirmation, as one JSON object.
func postFinalisation(t *testing.T, layout state.Layout, ran ...summaryOwner) string {
	t.Helper()
	body, err := layout.ReadRound(draftOwner, draftRepo, draftPRNum, draftRound, state.FileSummary)
	require.NoError(t, err)
	document := assertSummaryShape(t, body, ran...)
	share := map[string]json.RawMessage{}
	for _, key := range []string{summaryComments, summaryPosted, summaryPayloadHash, summaryConfirmGiven} {
		if raw, held := document[key]; held {
			share[key] = raw
		}
	}
	finalised, err := json.Marshal(share)
	require.NoError(t, err)
	return string(finalised)
}

// §10.3 over a confirmed post that discards records the draft still queued: the
// summary's comment count is the one the review carried, not the draft's, so it
// agrees with the posted count beside it. QA D-W2-1 saw comments.count 10 beside
// posted 8.
func TestAConfirmedPostRecordsTheCommentCountItsReviewCarried(t *testing.T) {
	kept, deleted, wrong := aCitedRecord("f1"), aCitedRecord("f2"), aCitedRecord("f3")
	deleted.Anchor.Path = "internal/api/deleted.go"
	wrong.Anchor.Path = "internal/api/wrong.go"
	layout := draftedHome(t, kept, deleted, wrong)
	redraft(t)
	assert.JSONEq(t, `{"comments":{"count":3,"max":20}}`, postFinalisation(t, layout, ownerDraft, ownerComments),
		"the draft queued three comments")
	edited := deleteBlock(t, readDraft(t, layout), "f2")
	writeDraft(t, layout, markerEdit(t, edited, "f3", `disposition=""`, `disposition="wrong"`))
	payload := builtPayload(t)
	require.Len(t, payload.Comments, 1)
	ghShimming(t, payload)

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)

	hash, err := payload.Hash()
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"comments":{"count":1,"max":20},"posted":1,"payload_hash":"`+hash+`","confirm_given":true}`,
		postFinalisation(t, layout, ownerDraft, ownerComments, ownerDiscards, ownerPost))
}

// §8.5.4 and §10.3 over a confirmed post whose draft discards every queued
// record: nothing is sent, and the summary still records that --confirm was
// given, no comment and no posted record, and an empty payload hash for the
// payload that was never built. The round is not thereby posted: a second run
// is refused as holding nothing to post, and --reconcile reports the empty
// hash. QA D-W2-4 saw none of the three keys written.
func TestAnAllDiscardedConfirmedPostFinalisesTheRoundSummary(t *testing.T) {
	deleted, wrong := aCitedRecord("f1"), aCitedRecord("f2")
	deleted.Anchor.Path = "internal/api/deleted.go"
	wrong.Anchor.Path = "internal/api/wrong.go"
	layout := draftedHome(t, deleted, wrong)
	redraft(t)
	edited := deleteBlock(t, readDraft(t, layout), "f1")
	writeDraft(t, layout, markerEdit(t, edited, "f2", `disposition=""`, `disposition="wrong"`))
	shim := ghShimming(t, &post.Review{})

	_, err := runPost(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)
	assert.JSONEq(t, `{"comments":{"count":2,"max":20}}`, postFinalisation(t, layout, ownerDraft, ownerComments),
		"§8.5.1: the dry run finalises nothing")

	_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)
	assert.Empty(t, shim.writes(t))
	assert.JSONEq(t,
		`{"comments":{"count":0,"max":20},"posted":0,"payload_hash":"","confirm_given":true}`,
		postFinalisation(t, layout, ownerDraft, ownerComments, ownerDiscards, ownerPost))

	_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	var empty *EmptyReviewError
	require.ErrorAs(t, err, &empty, "a round that sent no review is not refused as posted")
	assert.Equal(t, EmptyReviewError{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum, Round: draftRound, Discarded: 2,
	}, *empty)

	printed, err := runPost(t, draftPR, "--repo", draftSlug, "--reconcile")
	require.NoError(t, err)
	var reconciled struct {
		PayloadHash *string `json:"payload_hash"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &reconciled))
	require.NotNil(t, reconciled.PayloadHash)
	assert.Empty(t, *reconciled.PayloadHash)
	assert.Empty(t, shim.writes(t))
}
