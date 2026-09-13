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

// cappedDraft is draftedHome's round holding records, drafted under a
// repository-layer post.max_comments of maxComments, with a gh shim on PATH
// recording every invocation.
func cappedDraft(t *testing.T, maxComments string, records ...*finding.Finding) (state.Layout, *ghShimTranscript) {
	t.Helper()
	layout := draftedHome(t, records...)
	require.NoError(t, layout.EnsureRepo(draftOwner, draftRepo))
	require.NoError(t, os.WriteFile(layout.RepoConfig(draftOwner, draftRepo),
		[]byte(`{"post": {"max_comments": `+maxComments+`}}`), 0o600))
	redraft(t)
	return layout, ghShimming(t, &post.Review{})
}

// §1.6.2 through `cr post`: a round one comment over post.max_comments is
// refused with exit code 1, naming the count and the cap, whether or not
// --confirm was given, and nothing reaches GitHub.
//
// The refusal is asked of the command rather than of finding.CommentCap, which
// is how the cap went unenforced once: the decision was tested and nothing
// called it. The shim is on PATH for both runs, so a confirmed run that got
// past the cap would have had somewhere to send its review; that it received
// no POST is the network half of the assertion, and the records still sitting
// in `queued` is the state half.
func TestPostingOverTheCommentCapIsRefusedWithAndWithoutConfirm(t *testing.T) {
	layout, shim := cappedDraft(t, "2", aCitedRecord("f1"), aCitedRecord("f2"), aCitedRecord("f3"))

	for _, flags := range [][]string{nil, {"--confirm"}} {
		_, err := runPost(t, append([]string{draftPR, "--repo", draftSlug}, flags...)...)

		// assert rather than require, so a run that got past the cap
		// without --confirm still leaves the confirmed run its turn at
		// the shim below.
		var exceeded *finding.CommentCapExceededError
		if !assert.ErrorAs(t, err, &exceeded, "flags %v", flags) {
			continue
		}
		assert.Equal(t, finding.CommentCap{Count: 3, Max: 2}, exceeded.Cap)
		assert.Equal(t, ExitValidation, exitCodeFor(err), "§1.6.2 blocks with exit code 1")
		assert.Contains(t, err.Error(), "3 comments queued against post.max_comments 2, 1 over the cap",
			"the refusal names the count and the cap")
		assert.Contains(t, hintFor(err), "discard records in the draft", "and hints at triaging to fit")
	}

	assert.Empty(t, shim.writes(t), "§8.5: a refused round sends nothing, confirmed or not")
	stored, err := state.ReadStamped[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings, draftRound)
	require.NoError(t, err)
	require.Len(t, stored, 3)
	for i := range stored {
		assert.Equal(t, finding.StateQueued, stored[i].State, "§1.6.2: nothing is dropped or posted to fit")
	}
}

// The cap is the last count that fits: a round at exactly post.max_comments is
// not refused, and the dry run prints both comments and sends nothing.
func TestPostingAtExactlyTheCommentCapProceeds(t *testing.T) {
	_, shim := cappedDraft(t, "2", aCitedRecord("f1"), aCitedRecord("f2"))

	printed, err := runPost(t, draftPR, "--repo", draftSlug)

	require.NoError(t, err)
	var report dryRun
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	require.NotNil(t, report.Payload)
	assert.Len(t, report.Payload.Comments, 2, "both comments are carried, none dropped")
	assert.False(t, report.Posted)
	assert.Empty(t, shim.writes(t), "§8.5.1: the dry run sends nothing")
}
