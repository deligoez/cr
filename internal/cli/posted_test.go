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
// the network call, and the path it was written to is what the call sends.
//
// The file is asserted to be the request body byte for byte, because that is
// the whole point of writing it first: post.Request sends it with `--input`, so
// a document carrying anything of cr's own would put a field GitHub never named
// into the request, and a document that merely described the payload would let
// the bytes written and the bytes sent differ.
func TestThePayloadIsOnDiskBeforeTheCallAndIsWhatTheCallSends(t *testing.T) {
	layout := draftedHome(t)
	review := aPostedReview()

	path, err := writePosted(layout, draftOwner, draftRepo, draftPRNum, draftRound, review)
	require.NoError(t, err)
	assert.Equal(t,
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FilePosted), path,
		"§8.3.3 names rounds/<n>/posted.json, and post.Request sends that file")

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	payload, err := review.Payload()
	require.NoError(t, err)
	assert.Equal(t, string(payload)+"\n", string(written),
		"the bytes on disk are the payload, not a description of it")

	assert.Contains(t, post.Request(draftOwner, draftRepo, draftPRNum, path), path,
		"the call reads its body from the file that was just written")
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
