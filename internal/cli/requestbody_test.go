package cli

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/state"
)

// membersOf is a JSON document's top-level member names, sorted.
func membersOf(t *testing.T, document []byte) []string {
	t.Helper()
	var members map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(document, &members))
	return slices.Sorted(maps.Keys(members))
}

// §8.3.3 against the request itself: posted.json holds the exact payload with
// cr's own sections beside it, and the call to GitHub carries the payload and
// nothing else.
//
// The round is triagedRound's, so every section cr writes into posted.json
// before the call is populated: the two records the comments came from, the
// two discards, and an outcome for each of the four records carrying its
// kind, grade, severity and anchor — the discarded-wrong one included. The body
// is read at the shim, which copies whatever gh would have sent, so the
// assertion is about the request and not about how cr chose to hand it over.
func TestTheReviewRequestCarriesOnlyTheReviewPayload(t *testing.T) {
	layout := triagedRound(t)
	built := builtPayload(t)
	shim := ghShimming(t, built)

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)
	require.Len(t, shim.writes(t), 1, "§8.3.1: the round is one review")

	payload, err := built.Payload()
	require.NoError(t, err)
	sent := shim.sentBody(t)
	assert.Equal(t, []string{"body", "comments", "event"}, membersOf(t, sent),
		"the request carries GitHub's review fields and none of cr's own sections")
	assert.Equal(t, membersOf(t, payload), membersOf(t, sent),
		"the request's members are exactly the review payload's")
	assert.Equal(t, string(payload)+"\n", string(sent),
		"§8.3.3: the request body is the payload posted.json was written with, byte for byte")

	document, err := os.ReadFile(
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FilePosted))
	require.NoError(t, err)
	assert.Equal(t,
		[]string{"body", "comments", "discards", "event", "outcomes", "records", "threads"},
		membersOf(t, document),
		"posted.json keeps the payload and every section §8.4.4's adoption reads")
	stored, err := post.Decode(document)
	require.NoError(t, err)
	assert.Equal(t, []string{"f1", "f2"}, stored.Records)
	assert.Equal(t, []post.Discard{
		{Record: "f3", Disposition: finding.DispositionNotHere},
		{Record: "f4", Disposition: finding.DispositionWrong},
	}, stored.Discards)
	outcomes := make([]string, 0, len(stored.Outcomes))
	for i := range stored.Outcomes {
		outcomes = append(outcomes, stored.Outcomes[i].Record+":"+string(stored.Outcomes[i].Outcome))
	}
	assert.Equal(t, []string{
		"f1:kept", "f2:softened", "f3:discarded-not-here", "f4:discarded-wrong",
	}, outcomes)
}
