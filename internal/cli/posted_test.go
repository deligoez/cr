package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/state"
)

// aPostedReview is the review a round would send: two comments and the review
// body §8.4.3 puts the disclosure and the hash in.
func aPostedReview() *post.Review {
	records := []*finding.Finding{
		aStoredRecord("f1", finding.StateQueued),
		aStoredRecord("f2", finding.StateQueued),
	}
	return post.Build(records, map[string]string{
		"f1": "The error is dropped.",
		"f2": "Is the rounding deliberate?",
	})
}

// readPosted reads the round's posted.json back as a raw document, so a field
// this test does not name is still visible to it.
func readPosted(t *testing.T, layout state.Layout) map[string]json.RawMessage {
	t.Helper()
	body, err := os.ReadFile(
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FilePosted))
	require.NoError(t, err)
	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &document))
	return document
}

// §8.3.3: the exact posted payload is written to rounds/<n>/posted.json before
// the network call, and the bytes that write produced are what the call sends.
//
// The file is asserted to hold the payload byte for byte, and the bytes handed
// back to be the same bytes, because that is the whole point of writing it
// first: a document that merely described the payload would let the bytes
// written and the bytes sent differ.
func TestThePayloadIsOnDiskBeforeTheCallAndIsWhatTheCallSends(t *testing.T) {
	layout := draftedHome(t)
	review := aPostedReview()

	sent, err := writePosted(layout, draftOwner, draftRepo, draftPRNum, draftRound, review)
	require.NoError(t, err)

	written, err := os.ReadFile(
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FilePosted))
	require.NoError(t, err)
	payload, err := review.Payload()
	require.NoError(t, err)
	assert.Equal(t, string(payload)+"\n", string(written),
		"the bytes on disk are the payload, not a description of it")
	assert.Equal(t, string(written), string(sent),
		"the bytes the call sends are the bytes that were written")
}

// §8.3.3: posted.json is updated with the returned thread ids after the call,
// and the payload it was written with survives the update.
//
// The surviving fields are the assertion beside the new one. §2.3 calls this
// file "the exact payload posted for round n", so an update that re-encoded the
// document from a struct of its own would leave the round with thread ids and
// no record of what they are threads on.
func TestTheReturnedThreadIdsJoinThePayloadRatherThanReplacingIt(t *testing.T) {
	layout := draftedHome(t)
	review := aPostedReview()
	_, err := writePosted(layout, draftOwner, draftRepo, draftPRNum, draftRound, review)
	require.NoError(t, err)

	require.NoError(t, adoptThreads(
		layout, draftOwner, draftRepo, draftPRNum, draftRound,
		map[string]string{"f1": "PRRT_kwDOAbCd", "f2": "PRRT_kwDOEfGh"},
	))

	document := readPosted(t, layout)
	var threads map[string]string
	require.NoError(t, json.Unmarshal(document[postedThreads], &threads))
	assert.Equal(t, map[string]string{"f1": "PRRT_kwDOAbCd", "f2": "PRRT_kwDOEfGh"}, threads,
		"§8.3.3: the ids GitHub returned, keyed by the record each comment came from")

	var comments []post.Comment
	require.NoError(t, json.Unmarshal(document["comments"], &comments))
	assert.Len(t, comments, 2, "the payload is still in the document the ids were added to")
	assert.JSONEq(t, `"COMMENT"`, string(document["event"]),
		"§8.3.2's event is still the one the round was posted with")
}

// readOnly makes dir unwritable for the rest of the test and writable again
// after it, so a §2.3 write into it fails on the temporary file it cannot
// create while every read of it still succeeds.
func readOnly(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root writes through a read-only directory")
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	require.NoError(t, os.Chmod(dir, 0o500))
}

// The three writes `cr post` makes after its network call report their own
// failure rather than swallowing it.
//
// gremlins found each guard unasserted: negated, the lock was released and the
// write's error dropped, so the caller was told nothing. What is lost differs.
// A posted index that did not land re-raises, next round, a comment the author
// already received — the failure §9.3.6 exists to prevent. The records section
// is what §8.4.4's reconciliation reads back to adopt a review whose outcome
// was unknown. The thread ids are §8.3.3's provenance. Each is driven directly,
// as the tests above drive writePosted and adoptThreads, with only the
// directory its write lands in made read-only.
func TestTheWritesAfterThePostReportTheirOwnFailure(t *testing.T) {
	t.Run("the posted index", func(t *testing.T) {
		layout := draftedHome(t)
		posted := aStoredRecord("f1", finding.StatePosted)
		dir := layout.PRDir(draftOwner, draftRepo, draftPRNum)
		readOnly(t, dir)

		err := recordPostedIndex(layout, aPostingRound(draftRound, draftHead), []*finding.Finding{posted})

		require.Error(t, err, "§9.3.6's entry did not land, and the caller has to know")
		assert.Contains(t, err.Error(), dir)
	})
	t.Run("the thread ids", func(t *testing.T) {
		layout := draftedHome(t)
		_, err := writePosted(layout, draftOwner, draftRepo, draftPRNum, draftRound, aPostedReview())
		require.NoError(t, err)
		dir := layout.RoundDir(draftOwner, draftRepo, draftPRNum, draftRound)
		readOnly(t, dir)

		err = adoptThreads(layout, draftOwner, draftRepo, draftPRNum, draftRound,
			map[string]string{"f1": "PRRT_kwDOAbCd"})

		require.Error(t, err, "§8.3.3's ids did not land, and the caller has to know")
		assert.Contains(t, err.Error(), dir)
	})
	t.Run("the records an unknown outcome was drawn from", func(t *testing.T) {
		layout := draftedHome(t,
			aStoredRecord("f1", finding.StateQueued), aStoredRecord("f2", finding.StateQueued))
		review := aPostedReview()
		_, err := writePosted(layout, draftOwner, draftRepo, draftPRNum, draftRound, review)
		require.NoError(t, err)
		round, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
		require.NoError(t, err)
		dir := layout.RoundDir(draftOwner, draftRepo, draftPRNum, draftRound)
		readOnly(t, dir)

		err = postOutcome(layout, &round, review, &gh.CommandError{
			Args: post.Request(draftOwner, draftRepo, draftPRNum),
			Err:  &exec.ExitError{},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), dir,
			"§8.4.4's reconciliation reads these ids back, so a write that failed is named")
		stored, readErr := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
		require.NoError(t, readErr)
		assert.True(t, stored.PostUnresolved, "meta.json is outside the directory that refused")
	})
}
