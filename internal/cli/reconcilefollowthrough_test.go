package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// §8.3.1 against §8.4.4, audit round 3's task-reconcile-adopts-thread-ids end
// to end: a send carrying a draft discard meets a `gh` that dies, and
// `cr post --reconcile` adopts the review and stores the discard its waiver
// describes, under the confirmed send's own §9.1 row. The reviewer then takes
// the discard back in the draft, and neither the dry run nor the confirmed run
// can reach the author with a second review: both are refused as posted, and
// nothing after the first send reaches the write door — so the waiver stands
// for a record that stays discarded, never for one posted under it.
func TestAnAdoptedRoundTakesNoSecondReviewWhateverTheDraftSays(t *testing.T) {
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
			untriaged := readDraft(t, layout)
			writeDraft(t, layout, verb.triage(t, untriaged))

			report, shim, embedded := sendWithUnknownOutcome(t)
			require.Equal(t, adoptedReviewURL, report.Adopted)
			require.Equal(t, []string{"f1"}, report.Records)

			stored := roundRecordsByID(t, layout)
			assert.Equal(t, finding.StatePosted, stored["f1"].State)
			assert.Equal(t, finding.StateDiscarded, stored["f2"].State,
				"§9.1: the adoption stores the discard the confirmed send read")
			assert.Equal(t, verb.disposition, stored["f2"].Disposition)
			assert.Equal(t, verb.scope, waiverScopeOf(t, layout, discarded.Anchor.Path))

			writeDraft(t, layout, untriaged)
			for _, flags := range [][]string{{}, {"--confirm"}} {
				_, err := runPost(t, append([]string{draftPR, "--repo", draftSlug}, flags...)...)

				assertRefusedAsPosted(t, err, 1, embedded)
			}
			assert.Len(t, shim.writes(t), 1, "§8.3.1: nothing after the first send reached the write door")
			stored = roundRecordsByID(t, layout)
			assert.Equal(t, finding.StatePosted, stored["f1"].State)
			assert.Equal(t, finding.StateDiscarded, stored["f2"].State)
			assert.Equal(t, verb.scope, waiverScopeOf(t, layout, discarded.Anchor.Path),
				"§7.4: the waiver stands beside the discard it describes")
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

// assertRefusedAsPosted holds err to §8.3.1's refusal of a second review for
// the fixture's round: the whole error, §11.2's state conflict, and the hint.
func assertRefusedAsPosted(t *testing.T, err error, posted int, hash string) {
	t.Helper()
	var refused *PostedRoundError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, PostedRoundError{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum, Round: draftRound,
		Posted: posted, PayloadHash: hash,
	}, *refused)
	assert.Equal(t, ExitState, exitCodeFor(err))
	assert.Equal(t,
		"this round is posted and takes no second review; `cr status <pr>` reports where its "+
			"records stand, and `cr brief <pr>` opens the next round once the head moves",
		hintFor(err))
}

// §8.3.1: once `cr post --confirm` has created a round's review, a second run
// sends no second review, whatever the draft now says. The reviewer clears the
// `wrong` the first send stored, which leaves the draft asking for a comment
// the round has not posted — and both the dry run and the confirmed run are
// refused as posted, before the gate, with no invocation after the first
// carrying a request body to the `gh` on PATH. The discard and its waiver stand
// together, as the first send stored them.
func TestConfirmOnAPostedRoundSendsNoSecondReview(t *testing.T) {
	kept, discarded := aCitedRecord("f1"), aCitedRecord("f2")
	discarded.Anchor.Path = "internal/api/discarded.go"
	layout := draftedHome(t, kept, discarded)
	redraft(t)
	untriaged := readDraft(t, layout)
	writeDraft(t, layout, markerEdit(t, untriaged, "f2", `disposition=""`, `disposition="wrong"`))
	built := builtPayload(t)
	hash, err := built.Hash()
	require.NoError(t, err)
	shim := ghShimming(t, built)
	_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)
	require.Len(t, shim.writes(t), 1)

	writeDraft(t, layout, untriaged)
	for _, flags := range [][]string{{"--confirm"}, {}} {
		_, err = runPost(t, append([]string{draftPR, "--repo", draftSlug}, flags...)...)

		assertRefusedAsPosted(t, err, 1, hash)
		assert.Len(t, shim.writes(t), 1, "§8.3.1: the round's one review was already sent")
	}
	assert.Equal(t, finding.StateDiscarded, roundRecordsByID(t, layout)["f2"].State)
	assert.Equal(t, "repository", waiverScopeOf(t, layout, discarded.Anchor.Path))
}
