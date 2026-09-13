package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// sendWithUnknownOutcome runs `cr post --confirm` against a `gh` that dies on
// the review-creation call, then `cr post --reconcile` against the same `gh`,
// which lists a review carrying the payload's own body. It returns the payload
// the dry run built, the reconciliation's report, and the shim's transcript.
func sendWithUnknownOutcome(t *testing.T) (*reconcileResult, *ghShimTranscript, string) {
	t.Helper()
	payload := builtPayload(t)
	shim := reconcilingShim(t, payload)
	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.Error(t, err, "§8.4.4: an outcome cr could not establish is not a success")
	require.Len(t, shim.writes(t), 1)

	printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
	require.NoError(t, err)
	report := reconcileReport(t, printed)
	embedded, found := render.PayloadHashIn(payload.Body)
	require.True(t, found)
	return &report, shim, embedded
}

// §8.4.4 against §9.1 and §7.2: a round adopted by `cr post --reconcile`
// after a send that carried a draft discard is not left stuck. The adoption
// moves only what §9.1 lets it move, `queued` to `posted`; the discard is
// stored by the next `cr draft`, whose row §9.1 lists, reading the same draft
// the send read. A block for a record the round has posted names a record the
// round rendered, so neither `cr draft` nor the dry run refuses it as unknown.
func TestAnAdoptedRoundWithADiscardIsNotLeftStuck(t *testing.T) {
	for name, verb := range map[string]struct {
		triage      func(t *testing.T, file string) string
		disposition finding.Disposition
		scope       string
	}{
		"marked wrong": {
			triage: func(t *testing.T, file string) string {
				t.Helper()
				return markerEdit(t, file, "f2", `disposition=""`, `disposition="wrong"`)
			},
			disposition: finding.DispositionWrong, scope: "repository",
		},
		"deleted": {
			triage: func(t *testing.T, file string) string {
				t.Helper()
				return deleteBlock(t, file, "f2")
			},
			disposition: finding.DispositionNotHere, scope: "pull request",
		},
	} {
		t.Run(name, func(t *testing.T) {
			kept, discarded := aCitedRecord("f1"), aCitedRecord("f2")
			discarded.Anchor.Path = "internal/api/discarded.go"
			layout := draftedHome(t, kept, discarded)
			redraft(t)
			writeDraft(t, layout, verb.triage(t, readDraft(t, layout)))

			report, shim, _ := sendWithUnknownOutcome(t)
			require.Equal(t, adoptedReviewURL, report.Adopted)
			require.Equal(t, []string{"f1"}, report.Records)

			// The dry run reads the draft's discard and finds nothing left
			// to send, which is the refusal it owes, and not §7.2.3's.
			_, err := runPost(t, draftPR, "--repo", draftSlug)
			var marker *draft.MarkerEditError
			assert.NotErrorAs(t, err, &marker,
				"§7.2: a posted record's block names a record this round rendered")
			var empty *EmptyReviewError
			require.ErrorAs(t, err, &empty)
			assert.Equal(t, EmptyReviewError{
				Owner: draftOwner, Repo: draftRepo, PR: draftPRNum, Round: draftRound,
				Posted: 1, Discarded: 1,
			}, *empty)
			assert.Equal(t, finding.StateQueued, roundRecordsByID(t, layout)["f2"].State,
				"§8.5.1: the dry run stores nothing")

			_, err = runDraft(t, draftPR, "--repo", draftSlug)
			require.NoError(t, err)

			stored := roundRecordsByID(t, layout)
			assert.Equal(t, finding.StatePosted, stored["f1"].State)
			assert.Equal(t, finding.StateDiscarded, stored["f2"].State,
				"§9.1: `cr draft` stores the discard the adopted send read")
			assert.Equal(t, verb.disposition, stored["f2"].Disposition)
			assert.Equal(t, verb.scope, waiverScopeOf(t, layout, discarded.Anchor.Path))
			assert.Len(t, shim.writes(t), 1, "nothing after the first send reached the write door")
		})
	}
}

// §8.3.3's record-id tie-break, read back by §8.4.4: two comments sharing path,
// start_line, line and side are ordered by record id in the pre-image, and the
// hash `cr post --reconcile` recomputes from posted.json is the hash the send
// embedded — so the review that carries it is adopted rather than the round
// being released for a second send.
//
// The records are stored f2 before f1, so the payload holds them in that order
// and only the tie-break puts f1 first.
func TestReconcileMatchesTheHashOfTwoCommentsAtOnePosition(t *testing.T) {
	later, earlier := aCitedRecord("f2"), aCitedRecord("f1")
	earlier.Class = "shadowed-error"
	layout := draftedHome(t, later, earlier)
	redraft(t)

	report, _, embedded := sendWithUnknownOutcome(t)

	assert.Equal(t, embedded, report.PayloadHash,
		"§8.3.3: the recomputed hash is the embedded one")
	assert.Equal(t, adoptedReviewURL, report.Adopted)
	assert.Equal(t, []string{"f2", "f1"}, report.Records)
	assert.False(t, report.Unresolved)
	stored := roundRecordsByID(t, layout)
	assert.Equal(t, finding.StatePosted, stored["f1"].State)
	assert.Equal(t, finding.StatePosted, stored["f2"].State)
}

// §8.3.1 and §8.4.4: once every record of a round is posted, a second
// `cr post --confirm` sends no second review holding no comment. It exits with
// §11.2's state conflict, and no invocation after the first carries a request
// body to the `gh` on PATH. The dry run refuses the same way, since the
// refusal stands before the gate.
func TestConfirmOnAPostedRoundSendsNoEmptyReview(t *testing.T) {
	draftedHome(t, aCitedRecord("f1"))
	redraft(t)
	shim := ghShimming(t, builtPayload(t))
	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)
	require.Len(t, shim.writes(t), 1)

	for _, flags := range [][]string{{"--confirm"}, {}} {
		_, err = runPost(t, append([]string{draftPR, "--repo", draftSlug}, flags...)...)

		var empty *EmptyReviewError
		require.ErrorAs(t, err, &empty, "flags %v", flags)
		assert.Equal(t, EmptyReviewError{
			Owner: draftOwner, Repo: draftRepo, PR: draftPRNum, Round: draftRound, Posted: 1,
		}, *empty)
		assert.Equal(t, ExitState, exitCodeFor(err))
		assert.Equal(t,
			"nothing in this round is left to post; `cr status <pr>` reports where its records "+
				"stand, `cr draft <pr>` queues records recorded since, and `cr brief <pr>` opens "+
				"the next round once the head moves",
			hintFor(err))
		assert.Len(t, shim.writes(t), 1, "§8.3.1: the round's one review was already sent")
	}
}
