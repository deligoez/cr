package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/state"
)

// adoptedReviewURL is the url the reconciling shim lists cr's review under.
const adoptedReviewURL = "https://github.com/acme/web/pull/7#pullrequestreview-9"

// reconcilingShim installs a `gh` on PATH that records every invocation, lists
// one review whose body is review's own — so it carries §8.4.3's embedded
// payload hash — answers §3.5.1's thread query with one thread per comment of
// review, and fails the review-creation call the way a killed process does: a
// non-zero exit and nothing on standard output, which is §8.4.4's unknown
// outcome rather than §8.4.2's refusal.
func reconcilingShim(t *testing.T, review *post.Review) *ghShimTranscript {
	t.Helper()
	dir := t.TempDir()
	transcript := filepath.Join(dir, "transcript")
	threads := filepath.Join(dir, "threads.json")
	reviews := filepath.Join(dir, "reviews.json")

	require.NoError(t, os.WriteFile(threads, threadsPage(t, review), 0o600))
	page, err := json.Marshal(map[string]any{"data": map[string]any{
		"repository": map[string]any{"pullRequest": map[string]any{
			"reviews": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
				"nodes": []map[string]string{
					{"id": "PRR_other", "url": adoptedReviewURL + "0", "body": "looks good to me"},
					{"id": "PRR_ours", "url": adoptedReviewURL, "body": review.Body},
				},
			},
		}},
	}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(reviews, page, 0o600))

	shim := filepath.Join(dir, "gh")
	require.NoError(t, os.WriteFile(shim, []byte(
		"#!/bin/sh\n"+
			"printf '%s\\n' \"$*\" >> "+transcript+"\n"+
			"case \"$*\" in\n"+
			"  *reviewThreads*) exec cat "+threads+" ;;\n"+
			"  *'reviews('*) exec cat "+reviews+" ;;\n"+
			"  *'--method POST'*) exit 1 ;;\n"+
			"  *) echo '{}' ;;\n"+
			"esac\n"), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &ghShimTranscript{path: transcript}
}

// roundRecordsByID are the round's records as findings.ndjson holds them, keyed by
// id.
func roundRecordsByID(t *testing.T, layout state.Layout) map[string]*finding.Finding {
	t.Helper()
	stored, err := state.ReadStamped[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings, draftRound)
	require.NoError(t, err)
	byID := make(map[string]*finding.Finding, len(stored))
	for i := range stored {
		byID[stored[i].ID] = &stored[i]
	}
	return byID
}

// §8.4.4 and §8.3.3 together, end to end through `cr post`: a confirmed send
// whose outcome cr never learned is adopted by `cr post --reconcile`, and the
// adoption does what the confirmed send would have done once its call returned.
// Each adopted record carries the thread its comment became, in findings.ndjson
// and in posted.json alike, and the draft's discard is waived in the scope its
// disposition names.
//
// The send meets a `gh` that dies without answering, so nothing about the
// adoption is seeded by hand: posted.json, `post_unresolved` and the discards
// named beside the payload are what the real send wrote. The shim counts every
// invocation carrying a request body, and the reconciliation adds none —
// posting twice is the worse failure.
func TestReconcileAdoptsTheThreadIDsAndTheWaiversAConfirmedSendOwed(t *testing.T) {
	first, second, discarded := aCitedRecord("f1"), aCitedRecord("f2"), aCitedRecord("f3")
	second.Anchor.Path = "internal/api/second.go"
	discarded.Anchor.Path = "internal/api/discarded.go"
	layout := draftedHome(t, first, second, discarded)
	redraft(t)
	writeDraft(t, layout,
		markerEdit(t, readDraft(t, layout), "f3", `disposition=""`, `disposition="wrong"`))
	payload := builtPayload(t)
	shim := reconcilingShim(t, payload)

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.Error(t, err, "§8.4.4: an outcome cr could not establish is not a success")
	meta, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.True(t, meta.PostUnresolved)
	require.Len(t, shim.writes(t), 1)
	assert.Empty(t, waiverScopeOf(t, layout, discarded.Anchor.Path),
		"a call that may have been refused leaves no waiver behind")

	printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
	require.NoError(t, err)

	report := reconcileReport(t, printed)
	assert.Equal(t, adoptedReviewURL, report.Adopted)
	assert.Equal(t, []string{"f1", "f2"}, report.Records)
	assert.Len(t, shim.writes(t), 1, "§8.4.4: the reconciliation reads and never sends")

	stored := roundRecordsByID(t, layout)
	require.Len(t, stored, 3)
	threads := map[string]string{"f1": threadIDFor(0), "f2": threadIDFor(1)}
	for id, thread := range threads {
		assert.Equal(t, finding.StatePosted, stored[id].State, id)
		assert.Equal(t, thread, stored[id].ThreadID,
			"§6.1: %s carries the thread its comment became", id)
	}
	assert.Equal(t, finding.StateDiscarded, stored["f3"].State,
		"§9.1: the adoption stores the discard the confirmed send read")
	assert.Equal(t, finding.DispositionWrong, stored["f3"].Disposition)
	assert.Empty(t, stored["f3"].ThreadID)
	assert.Equal(t, []string{
		"f3:queued>discarded by cr post --confirm",
		"f1:queued>posted by cr post --reconcile",
		"f2:queued>posted by cr post --reconcile",
	}, journalOf(t, layout, draftJournal)[3:],
		"§9.1.1: the discard is the confirmed send's move, and only the posting is the adoption's")

	var inPosted map[string]string
	require.NoError(t, json.Unmarshal(readPosted(t, layout)[postedThreads], &inPosted))
	assert.Equal(t, threads, inPosted, "§8.3.3: posted.json keys the same ids by the same records")

	assert.Equal(t, "repository", waiverScopeOf(t, layout, discarded.Anchor.Path),
		"§7.4.1: the adopted round's `wrong` is waived across the repository")
	assert.Empty(t, waiverScopeOf(t, layout, first.Anchor.Path))
	assert.Empty(t, waiverScopeOf(t, layout, second.Anchor.Path))
}

// §7.4 against §8.4.2: a waiver for the draft's discard is written only once
// the review has been created, so a review GitHub refuses leaves none behind —
// and a `wrong` the reviewer then clears does not go on silencing its class
// across the repository after the record is posted.
//
// Audit round 2's waiver-before-rejectable-send, end to end: the first send
// meets a `gh` that refuses the review, the reviewer clears the disposition in
// the draft, and the second send meets a `gh` that accepts it.
func TestARejectedPostLeavesNoWaiverForADispositionClearedBeforeTheRetry(t *testing.T) {
	kept, retriaged := aCitedRecord("f1"), aCitedRecord("f2")
	retriaged.Anchor.Path = "internal/api/second.go"
	layout := draftedHome(t, kept, retriaged)
	redraft(t)
	writeDraft(t, layout,
		markerEdit(t, readDraft(t, layout), "f2", `disposition=""`, `disposition="wrong"`))
	rejectingShim(t)

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.Error(t, err)
	require.Equal(t, ExitState, exitCodeFor(err), "§8.4.2 exits 4")
	assert.Empty(t, waiverScopeOf(t, layout, retriaged.Anchor.Path),
		"§8.4.2: a refused review leaves the discard's waiver unwritten")
	assert.Equal(t, finding.StateQueued, roundRecordsByID(t, layout)["f2"].State)

	writeDraft(t, layout,
		markerEdit(t, readDraft(t, layout), "f2", `disposition="wrong"`, `disposition=""`))
	accepted := ghShimming(t, builtPayload(t))

	_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)

	require.Len(t, accepted.writes(t), 1)
	assert.Equal(t, finding.StatePosted, roundRecordsByID(t, layout)["f2"].State)
	wide, err := finding.RepositoryWaivers(layout, draftOwner, draftRepo)
	require.NoError(t, err)
	assert.Empty(t, wide, "no repository waiver outlives the disposition the reviewer cleared")
	assert.Empty(t, waiverScopeOf(t, layout, retriaged.Anchor.Path))
}
