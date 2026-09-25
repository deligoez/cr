package cli

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §8.3.1 through the dry run: once a round's review is created, `cr post`
// without `--confirm` is refused as well, and the message names the pull
// request, the round, the payload hash the round was posted as and how many of
// its records are posted — what a reader needs to find the review it means.
func TestAPostedRoundRefusesTheDryRunNamingItsReview(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"))
	redraft(t)
	shim := ghShimming(t, builtPayload(t))
	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)
	hash, recorded, err := state.ReadRoundSection[string](
		layout, draftOwner, draftRepo, draftPRNum, draftRound, state.FileSummary, summaryPayloadHash)
	require.NoError(t, err)
	require.True(t, recorded)
	require.NotEmpty(t, hash)

	_, err = runPost(t, draftPR, "--repo", draftSlug)

	require.Error(t, err)
	assert.Equal(t, fmt.Sprintf("%s/%s#%d round %d is posted: its review was created or adopted "+
		"(payload hash %q, 1 record(s) posted), and §8.3.1 posts a round's comments as one review, so cr "+
		"sends no second review for this round whatever its draft now says",
		draftOwner, draftRepo, draftPRNum, draftRound, hash), err.Error())
	assert.Equal(t, ExitState, exitCodeFor(err))
	assert.Len(t, shim.writes(t), 1, "the refused dry run sent nothing")
}
