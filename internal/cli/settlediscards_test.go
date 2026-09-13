package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/state"
)

// postReport is the document `cr post` prints, as far as these tests read it.
type postReport struct {
	Round        int             `json:"round"`
	Comments     []postedComment `json:"comments"`
	Payload      *post.Review    `json:"payload"`
	Discarded    []string        `json:"discarded"`
	Posted       bool            `json:"posted"`
	ConfirmGiven bool            `json:"confirm_given"`
}

// postReportOf parses a `cr post` run's printed document.
func postReportOf(t *testing.T, printed string) postReport {
	t.Helper()
	var report postReport
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	return report
}

// §7.2, §7.3.1 and §9.1 against §8.3.1, audit round 3's
// task-post-writes-triage-waivers: a draft that discards every queued record
// leaves nothing to post, so `cr post --confirm` builds no review and makes no
// network call, and settles what the send would have settled once its call
// returned — each discard stored under its disposition by the confirmed run's
// own §9.1 row, its waiver in the scope that disposition names, and one outcome
// event per queued record. The dry run reports the same discards and writes
// nothing, and a run after the settling finds nothing left to post.
func TestADraftDiscardingEveryRecordSettlesWithoutAReview(t *testing.T) {
	deleted, wrong := aCitedRecord("f1"), aCitedRecord("f2")
	deleted.Anchor.Path = "internal/api/deleted.go"
	wrong.Anchor.Path = "internal/api/wrong.go"
	layout := draftedHome(t, deleted, wrong)
	redraft(t)
	edited := deleteBlock(t, readDraft(t, layout), "f1")
	writeDraft(t, layout, markerEdit(t, edited, "f2", `disposition=""`, `disposition="wrong"`))
	shim := ghShimming(t, &post.Review{})
	before := journalOf(t, layout, draftJournal)

	printed, err := runPost(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err, "§8.5.1: the dry run exits 0")
	assert.Equal(t, postReport{
		Round: draftRound, Comments: []postedComment{}, Discarded: []string{"f1", "f2"},
	}, postReportOf(t, printed))
	stored := roundRecordsByID(t, layout)
	assert.Equal(t, finding.StateQueued, stored["f1"].State, "§8.5.1: the dry run stores nothing")
	assert.Equal(t, finding.StateQueued, stored["f2"].State)
	assert.Equal(t, []string{"f1:raised", "f2:raised"}, ledger(t, layout))
	assert.Empty(t, waiverScopeOf(t, layout, deleted.Anchor.Path))
	assert.Empty(t, waiverScopeOf(t, layout, wrong.Anchor.Path))

	printed, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)
	assert.Equal(t, postReport{
		Round: draftRound, Comments: []postedComment{}, Discarded: []string{"f1", "f2"},
		ConfirmGiven: true,
	}, postReportOf(t, printed), "§12.6: nothing was sent, and §8.5.4: --confirm was given")
	assert.Empty(t, shim.writes(t), "§8.3.1: no review holds no comment, so nothing is sent")

	stored = roundRecordsByID(t, layout)
	assert.Equal(t, finding.StateDiscarded, stored["f1"].State)
	assert.Equal(t, finding.DispositionNotHere, stored["f1"].Disposition)
	assert.Equal(t, finding.StateDiscarded, stored["f2"].State)
	assert.Equal(t, finding.DispositionWrong, stored["f2"].Disposition)
	assert.Equal(t, "pull request", waiverScopeOf(t, layout, deleted.Anchor.Path))
	assert.Equal(t, "repository", waiverScopeOf(t, layout, wrong.Anchor.Path))
	assert.Equal(t, []string{
		"f1:raised", "f2:raised", "f1:discarded-not-here", "f2:discarded-wrong",
	}, ledger(t, layout), "§7.3.1: one outcome event per queued record")
	assert.Equal(t, append(before,
		"f1:queued>discarded by cr post --confirm",
		"f2:queued>discarded by cr post --confirm",
	), journalOf(t, layout, draftJournal))
	for key, want := range map[string]int{summaryDiscardedNotHere: 1, summaryDiscardedWrong: 1} {
		count, recorded, err := state.ReadRoundSection[int](
			layout, draftOwner, draftRepo, draftPRNum, draftRound, state.FileSummary, key)
		require.NoError(t, err)
		assert.True(t, recorded, key)
		assert.Equal(t, want, count, key)
	}
	_, recorded, err := state.ReadRoundSection[string](
		layout, draftOwner, draftRepo, draftPRNum, draftRound, state.FileSummary, summaryPayloadHash)
	require.NoError(t, err)
	assert.False(t, recorded, "§10.3: a round that sent no payload records no payload hash")

	_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	var empty *EmptyReviewError
	require.ErrorAs(t, err, &empty)
	assert.Equal(t, EmptyReviewError{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum, Round: draftRound, Discarded: 2,
	}, *empty)
	assert.Equal(t, ExitState, exitCodeFor(err))
	assert.Empty(t, shim.writes(t))
}
