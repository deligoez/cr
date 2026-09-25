package cli

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/state"
)

// anUnresolvedPosting is the state §8.4.4 recovers from: a round whose payload
// was written, whose call went out, and whose outcome cr never learned.
//
// The outcome is produced rather than declared. The failure handed to
// postOutcome is a gh.CommandError carrying what a killed process leaves — a
// non-zero exit and nothing on standard output — which is the timeout case
// §8.4.4 names first, and post.Rejection answers nil for it because there is no
// error document to read. Setting the flag by hand would leave the fork between
// §8.4.2 and §8.4.4 untested, and that fork is the whole of criterion one.
//
// It returns the state root and the §8.4.3 hash of the payload that was sent.
// The payload names no commit_id, as a posted.json written before cr sent one
// does; anUnresolvedPostingAt writes one.
func anUnresolvedPosting(t *testing.T) (layout state.Layout, hash string) {
	t.Helper()
	return anUnresolvedPostingAt(t, "")
}

// anUnresolvedPostingAt is anUnresolvedPosting over a payload whose commit_id
// is commit.
func anUnresolvedPostingAt(t *testing.T, commit string) (layout state.Layout, hash string) {
	t.Helper()
	layout = draftedHome(t,
		aStoredRecord("f1", finding.StateQueued), aStoredRecord("f2", finding.StateQueued))
	review := aPostedReview()
	review.CommitID = commit
	_, err := writePosted(layout, draftOwner, draftRepo, draftPRNum, draftRound, review)
	require.NoError(t, err)

	round, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	timedOut := postOutcome(layout, &round, review, &gh.CommandError{
		Args: post.Request(draftOwner, draftRepo, draftPRNum),
		Err:  &exec.ExitError{},
	})
	require.Error(t, timedOut, "§8.4.4: an outcome cr could not establish is not a success")
	var rejected *post.RejectedError
	assert.NotErrorAs(t, timedOut, &rejected,
		"§8.4.2 is GitHub saying no, and nothing said no here")
	assert.Contains(t, timedOut.Error(), "--reconcile",
		"§8.4.4 does not retry, so the run names the move that is left")

	stored, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.True(t, stored.PostUnresolved, "§8.4.4 sets post_unresolved on meta.json")

	hash, err = review.Hash()
	require.NoError(t, err)
	return layout, hash
}

// reviewsAnswering installs a `gh` whose review listing is the one page given.
func reviewsAnswering(t *testing.T, reviews ...gh.Review) {
	t.Helper()
	page, err := json.Marshal(map[string]any{"data": map[string]any{
		"repository": map[string]any{"pullRequest": map[string]any{
			"reviews": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
				"nodes":    reviews,
			},
		}},
	}})
	require.NoError(t, err)

	restore := ghClient
	ghClient = func() gh.Client {
		return gh.WithRunner(func(_ ...string) (string, error) { return string(page), nil })
	}
	t.Cleanup(func() { ghClient = restore })
}

// postedIndexOf is `posted-index.ndjson` as it stands on disk.
func postedIndexOf(t *testing.T, layout state.Layout) []finding.PostedEntry {
	t.Helper()
	index, err := finding.PostedIndex(layout, draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	return index
}

// §8.4.4 end to end, in both directions: a posting whose outcome cr never
// learned is reconciled against the pull request's reviews, and the round is
// either adopted as posted or released for a retry.
//
// The two runs start from one fixture, so what separates them is the answer
// GitHub gives and nothing else. The index is the assertion that matters: it is
// written on adopt and not on clear, because a round that is going to be posted
// again must not carry entries that would drop its own findings at the next
// merge.
func TestAnUnresolvedPostingIsReconciledToAnAdoptAndToAClear(t *testing.T) {
	t.Run("no review carries the hash", func(t *testing.T) {
		layout, hash := anUnresolvedPosting(t)
		reviewsAnswering(t, gh.Review{
			ID: "PRR_other", URL: "https://github.com/acme/web/pull/7#pullrequestreview-1",
			Body: "looks good to me",
		})

		printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
		require.NoError(t, err)

		report := reconcileReport(t, printed)
		assert.Equal(t, hash, report.PayloadHash, "the hash matched on is the payload's own")
		assert.Empty(t, report.Adopted)
		assert.False(t, report.Unresolved, "§8.4.4 clears post_unresolved for a retry")
		assert.Empty(t, report.Records)

		stored, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
		require.NoError(t, err)
		assert.False(t, stored.PostUnresolved, "the flag is cleared on disk and not only in the report")
		assert.Empty(t, postedIndexOf(t, layout),
			"§9.3.6's entry stands for a comment the author received, and nobody received one")

		records, err := roundFindingsOf(layout, draftOwner, draftRepo, draftPRNum, draftRound)
		require.NoError(t, err)
		for _, record := range records {
			assert.Equal(t, finding.StateQueued, record.State,
				"§8.4.2 marks nothing posted, and neither does a reconciliation that found nothing")
		}
	})

	t.Run("a review carries the hash", func(t *testing.T) {
		layout, hash := anUnresolvedPosting(t)
		const url = "https://github.com/acme/web/pull/7#pullrequestreview-2"
		reviewsAnswering(t,
			gh.Review{ID: "PRR_other", URL: "…#pullrequestreview-1", Body: "looks good to me"},
			gh.Review{ID: "PRR_ours", URL: url, Body: reviewBodyCarrying(hash), Commit: gh.ReviewCommit{OID: draftHead}},
		)

		printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
		require.NoError(t, err)

		report := reconcileReport(t, printed)
		assert.Equal(t, url, report.Adopted, "§8.4.4 adopts the review carrying §8.4.3's hash")
		assert.False(t, report.Unresolved)
		assert.Equal(t, []string{"f1", "f2"}, report.Records)

		stored, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
		require.NoError(t, err)
		assert.False(t, stored.PostUnresolved)

		records, err := roundFindingsOf(layout, draftOwner, draftRepo, draftPRNum, draftRound)
		require.NoError(t, err)
		require.Len(t, records, 2)
		for _, record := range records {
			assert.Equal(t, finding.StatePosted, record.State,
				"§9.1's queued → posted row names `cr post --reconcile` as an actor")
		}

		// One entry, not two: both records of this fixture sit at one
		// anchor under one class, so §7.4.1's key is one key. §6.4.1
		// would have deduplicated them long before a payload, and the
		// index says the same thing from the other end — what it holds
		// is "this defect at this code was sent", once.
		index := postedIndexOf(t, layout)
		require.Len(t, index, 1, "§9.3.6 is keyed exactly as a §7.4.1 waiver is")
		assert.Equal(t, "f1", index[0].Record)
		assert.Equal(t, keyOf(t, aStoredRecord("f1", finding.StatePosted)), index[0].WaiverKey)
	})
}

// A second `--reconcile` over an adopted round changes nothing and refuses
// nothing.
//
// §9.1 lists no move out of `posted`, so a run that asked for one again would
// refuse the whole adoption on the strength of the first one having worked —
// which would leave the reviewer with a command that fails the moment it has
// already succeeded.
func TestReconcilingTwiceIsHarmless(t *testing.T) {
	layout, hash := anUnresolvedPosting(t)
	reviewsAnswering(t, gh.Review{ID: "PRR_ours", URL: "…#2", Body: reviewBodyCarrying(hash), Commit: gh.ReviewCommit{OID: draftHead}})

	require.NoError(t, runCLI(t, "post", draftPR, "--repo", draftSlug, "--reconcile"))
	require.NoError(t, runCLI(t, "post", draftPR, "--repo", draftSlug, "--reconcile"))

	assert.Len(t, postedIndexOf(t, layout), 1, "the second run appended nothing")
}

// §8.4.2 and §8.4.4 are one fork, and GitHub's error document is what tells
// them apart: a response cr can read is a refusal, and one it cannot is an
// outcome it does not know.
//
// The refusal is asserted to leave the flag alone, which is the half that
// matters: §8.4.2 marks nothing posted and there is nothing to reconcile, so a
// run that set `post_unresolved` here would send the reviewer to a recovery for
// a call that certainly did not post.
func TestARejectionIsNotAnUnknownOutcome(t *testing.T) {
	layout := draftedHome(t, aStoredRecord("f1", finding.StateQueued))
	review := aPostedReview()
	round, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)

	refused := postOutcome(layout, &round, review, &gh.CommandError{
		Stdout: `{"message":"Validation Failed","status":"422",` +
			`"errors":[{"field":"line","code":"invalid","message":"line must be part of the diff"}]}`,
		Err: &exec.ExitError{},
	})

	var rejected *post.RejectedError
	require.ErrorAs(t, refused, &rejected, "§8.4.2: GitHub answered, and it said no")
	assert.Equal(t, ExitState, exitCodeFor(refused), "§8.4.2 codes a rejected review 4")

	stored, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	assert.False(t, stored.PostUnresolved,
		"§8.4.4 is about an outcome cr could not establish, and this one is established")
}

// `cr post --reconcile` on a round with no unresolved posting reads the pull
// request and settles nothing.
//
// It is the case a reviewer reaches by habit, and the conservative answer is
// the only safe one: there is no outcome cr failed to learn, so there is
// nothing to adopt and nothing to clear.
func TestReconcilingARoundWithNothingUnresolvedSettlesNothing(t *testing.T) {
	layout := draftedHome(t, aStoredRecord("f1", finding.StateQueued))

	printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
	require.NoError(t, err)

	report := reconcileReport(t, printed)
	assert.Empty(t, report.PayloadHash)
	assert.Empty(t, report.Adopted)
	assert.False(t, report.Unresolved)
	assert.Empty(t, postedIndexOf(t, layout))
}

// reviewBodyCarrying is a review body with §8.4.3's hash embedded in it, built
// through render so the marker this is matched on is the marker cr writes.
func reviewBodyCarrying(hash string) string {
	return render.ReviewBody(render.LangEN, nil, nil, hash)
}

// reconcileReport reads the document `cr post --reconcile` printed.
func reconcileReport(t *testing.T, printed string) reconcileResult {
	t.Helper()
	var report reconcileResult
	require.NoError(t, json.Unmarshal([]byte(printed), &report),
		"the run printed %q", strings.TrimSpace(printed))
	require.NotEmpty(t, report.Honesty, "§9.3.1 is owed by every command that reads per-PR state")
	return report
}
