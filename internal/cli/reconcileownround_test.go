package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// The review QA's real-17 saw adopted for the wrong round: round 1's review on
// deligoez/cr-qa#15, as GitHub's review-creation call answered it, created at
// round 1's head.
const (
	roundOneReviewID  = "PRR_kwDOUJJL388AAAABNbrCHg"
	roundOneReviewURL = "https://github.com/deligoez/cr-qa/pull/15#pullrequestreview-5196399134"
	roundOneCommit    = "be7e2c75aeb661ba3c96d7a7634f3f8e84bb7b91"
)

// reviewNode is one review as GitHub's GraphQL review listing answers it.
func reviewNode(id, url, commit, body string) map[string]any {
	return map[string]any{"id": id, "url": url, "body": body, "commit": map[string]any{"oid": commit}}
}

// reviewListingShim installs a `gh` on PATH that records every invocation,
// lists nodes as the pull request's reviews, answers §3.5.1's thread query with
// the threads of every review in listed, and fails a review-creation call the
// way a killed process does.
func reviewListingShim(t *testing.T, listed []listedReview, nodes ...map[string]any) *ghShimTranscript {
	t.Helper()
	dir := t.TempDir()
	transcript := filepath.Join(dir, "transcript")
	threads := filepath.Join(dir, "threads.json")
	reviews := filepath.Join(dir, "reviews.json")

	require.NoError(t, os.WriteFile(threads, threadsPage(t, listed...), 0o600))
	page, err := json.Marshal(map[string]any{"data": map[string]any{
		"repository": map[string]any{"pullRequest": map[string]any{
			"reviews": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
				"nodes":    nodes,
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

// earlierReviewURL is the url of a review an earlier round of the fixture's pull
// request became, at the fixture round's own head.
const earlierReviewURL = "https://github.com/acme/web/pull/7#pullrequestreview-1"

// seedPostedIndex appends entries to `posted-index.ndjson` under the §2.3.1 lock.
func seedPostedIndex(t *testing.T, layout state.Layout, entries ...finding.PostedEntry) {
	t.Helper()
	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.NoError(t, finding.AppendPosted(held, layout, draftOwner, draftRepo, draftPRNum, entries))
	require.NoError(t, held.Unlock())
}

// anEntryOf is the index entry a record posted in round, in the review whose
// node id is review, leaves behind. Each round's record sits on a file of its
// own, so entries of different rounds never share §7.4.1's key.
func anEntryOf(round int, review string) finding.PostedEntry {
	posted := aStoredRecord("f9", finding.StatePosted)
	posted.Anchor.Path = "internal/api/round" + strconv.Itoa(round) + ".go"
	posted.Round, posted.Head = round, draftHead
	return finding.PostedEntryFor(posted, review)
}

// §8.4.4 adopts the review the round's own call may have created, and a review
// carrying the round's payload hash that is not that review is passed over,
// named with its reason, and `post_unresolved` is cleared for a retry.
//
// The first case is QA D-W2-5 replayed: round 2's payload hashes exactly as
// round 1's did, the call that set `post_unresolved` created nothing, and the
// only review carrying the hash is round 1's, as GitHub answered its creation,
// at round 1's head. Its threads carry the same bodies at the same places, so an
// adoption would key round 2's records onto threads the author already read.
func TestReconcilePassesOverAReviewThatIsNotTheRoundsOwn(t *testing.T) {
	for _, tc := range []struct {
		name   string
		index  []finding.PostedEntry
		review map[string]any
		want   notAdopted
	}{
		{
			name:   "a review created at another head",
			review: reviewNode(roundOneReviewID, roundOneReviewURL, roundOneCommit, ""),
			want: notAdopted{
				Review: roundOneReviewURL,
				Reason: "its commit " + roundOneCommit + " is not round 2's head " + draftHead,
			},
		},
		{
			name: "a review GitHub names no commit for",
			review: map[string]any{
				"id": roundOneReviewID, "url": roundOneReviewURL, "body": "", "commit": nil,
			},
			want: notAdopted{
				Review: roundOneReviewURL,
				Reason: "GitHub names no commit for it, so it is not shown to be at round 2's head " + draftHead,
			},
		},
		{
			name:   "a review the posted index holds for an earlier round",
			index:  []finding.PostedEntry{anEntryOf(draftRound-1, roundOneReviewID)},
			review: reviewNode(roundOneReviewID, earlierReviewURL, draftHead, ""),
			want: notAdopted{
				Review: earlierReviewURL,
				Reason: "the posted index already holds it for round 1",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout, hash := anUnresolvedPosting(t)
			seedPostedIndex(t, layout, tc.index...)
			tc.review["body"] = reviewBodyCarrying(hash)
			shim := reviewListingShim(t,
				[]listedReview{{id: roundOneReviewID, review: aPostedReview()}}, tc.review)

			printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
			require.NoError(t, err)

			report := reconcileReport(t, printed)
			assert.Equal(t, hash, report.PayloadHash)
			assert.Empty(t, report.Adopted, "§8.4.4 adopts this round's review, and this one is not")
			assert.Equal(t, []notAdopted{tc.want}, report.NotAdopted)
			assert.Equal(t, []string{}, report.Records)
			assert.False(t, report.Unresolved, "§8.4.4 clears post_unresolved for a retry")

			stored, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
			require.NoError(t, err)
			assert.False(t, stored.PostUnresolved, "the flag is cleared on disk and not only in the report")
			for id, record := range roundRecordsByID(t, layout) {
				assert.Equal(t, finding.StateQueued, record.State, id)
				assert.Empty(t, record.ThreadID, "%s is keyed onto no thread of that review", id)
			}
			assert.Equal(t, append([]finding.PostedEntry{}, tc.index...), postedIndexOf(t, layout),
				"§9.3.6's entry stands for a comment the author received in this round, and none was")
			assert.Empty(t, shim.writes(t), "reconciling sends nothing")

			// The document carries no field for having listed the
			// reviews, so the report is given it as the run held it.
			report.reconciled = true
			assert.Equal(t,
				"no review of round 2 carries payload hash "+hash+
					", so post_unresolved is cleared and the round may be posted again\n"+
					"review "+tc.want.Review+" carries payload hash "+hash+" and was not adopted: "+
					tc.want.Reason+"\n"+strings.Join(report.Honesty, "\n")+"\n",
				report.Text(&writer{}), "the terminal names the review and why it was not adopted")
		})
	}
}

// §8.4.4 over a pull request listing, oldest first, a review of round 1 at
// another head, a review the index holds for round 1 at this round's head, and
// this round's own review — which the index already holds for this round, as a
// reconciliation whose writes stopped after the index leaves it. The round's own
// review is adopted, its threads are the ones the records take, the two before
// it are named as passed over, and the index entries carry the adopted review.
func TestReconcileAdoptsTheRoundsOwnReviewPastOnesThatAreNot(t *testing.T) {
	layout, hash := anUnresolvedPosting(t)
	earlier, ours := anEntryOf(draftRound-1, "PRR_earlier"), anEntryOf(draftRound, adoptedReviewID)
	seedPostedIndex(t, layout, earlier, ours)
	body := reviewBodyCarrying(hash)
	reviewListingShim(t,
		[]listedReview{{id: roundOneReviewID, review: aPostedReview()}, {id: adoptedReviewID, review: aPostedReview()}},
		reviewNode(roundOneReviewID, roundOneReviewURL, roundOneCommit, body),
		reviewNode("PRR_earlier", earlierReviewURL, draftHead, body),
		reviewNode(adoptedReviewID, adoptedReviewURL, draftHead, body))

	printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
	require.NoError(t, err)

	report := reconcileReport(t, printed)
	assert.Equal(t, adoptedReviewURL, report.Adopted)
	assert.Equal(t, []string{"f1", "f2"}, report.Records)
	assert.Equal(t, []notAdopted{
		{Review: roundOneReviewURL, Reason: "its commit " + roundOneCommit + " is not round 2's head " + draftHead},
		{Review: earlierReviewURL, Reason: "the posted index already holds it for round 1"},
	}, report.NotAdopted)
	assert.False(t, report.Unresolved)

	stored := roundRecordsByID(t, layout)
	require.Len(t, stored, 2)
	for id, thread := range map[string]string{"f1": threadIDFor(2), "f2": threadIDFor(3)} {
		assert.Equal(t, finding.StatePosted, stored[id].State, id)
		assert.Equal(t, thread, stored[id].ThreadID, "%s takes the thread the adopted review opened", id)
	}
	posted := aStoredRecord("f1", finding.StatePosted)
	posted.Round, posted.Head = draftRound, draftHead
	assert.Equal(t, []finding.PostedEntry{earlier, ours, finding.PostedEntryFor(posted, adoptedReviewID)},
		postedIndexOf(t, layout), "§9.3.6's new entry names the review the record reached the author in")
}

// §9.3.6's entries written by a confirmed send name the review its call created,
// which is what a later round's reconciliation reads to pass that review over.
func TestAConfirmedSendIndexesTheReviewItCreated(t *testing.T) {
	record := aCitedRecord("f1")
	layout := draftedHome(t, record)
	redraft(t)
	ghShimming(t, builtPayload(t))

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)

	index := postedIndexOf(t, layout)
	require.Len(t, index, 1)
	assert.Equal(t, "f1", index[0].Record)
	assert.Equal(t, createdReviewID, index[0].Review)
}

// QA D-W2-3: `--reconcile` over a round that is already posted settles nothing,
// and reports the payload hash the round was posted under rather than an empty
// string that reads as a round with no payload.
func TestReconcilingAPostedRoundReportsItsPayloadHash(t *testing.T) {
	_, hash := anUnresolvedPosting(t)
	reviewListingShim(t, nil, reviewNode(adoptedReviewID, adoptedReviewURL, draftHead, reviewBodyCarrying(hash)))
	require.NoError(t, runCLI(t, "post", draftPR, "--repo", draftSlug, "--reconcile"))

	printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
	require.NoError(t, err)

	report := reconcileReport(t, printed)
	assert.Equal(t, hash, report.PayloadHash, "§8.5.4's round summary holds the hash the round was posted under")
	assert.Empty(t, report.Adopted, "this run adopted nothing")
	assert.Equal(t, []string{}, report.Records)
	assert.Equal(t, []notAdopted{}, report.NotAdopted)
	assert.False(t, report.Unresolved)
	assert.Equal(t,
		"round 2 has no unresolved posting to reconcile: it is posted under payload hash "+hash+"\n"+
			strings.Join(report.Honesty, "\n")+"\n",
		report.Text(&writer{}))
}
