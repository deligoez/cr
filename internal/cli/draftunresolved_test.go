package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// stateFiles is every file under the state root, keyed by its path relative to
// the root, with its bytes.
func stateFiles(t *testing.T, layout state.Layout) map[string]string {
	t.Helper()
	files := make(map[string]string)
	require.NoError(t, filepath.WalkDir(layout.Root(), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(layout.Root(), path)
		if err != nil {
			return err
		}
		files[rel] = string(raw)
		return nil
	}))
	return files
}

// §8.4.4 against §9.1, audit round 5's task-post-reconcile end to end: a
// confirmed send carrying f1 and f2 meets a `gh` that dies on the
// review-creation call, so the round carries post_unresolved. The reviewer then
// deletes f1's block and runs `cr draft`, which is refused as `cr post --confirm`
// is — exit 4, the reconciliation named — before it reads the draft or writes
// anything, so f1 stays queued rather than moving to a discarded state the
// adoption could never move to posted. `cr post --reconcile` then adopts the
// review the shim lists, and `cr draft` runs again once the posting is settled.
func TestADraftIsRefusedWhileAPostIsUnresolved(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
	redraft(t)
	shim := reconcilingShim(t, builtPayload(t))
	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.Error(t, err, "§8.4.4: an outcome cr could not establish is not a success")
	require.Len(t, shim.writes(t), 1)
	unsettled, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.True(t, unsettled.PostUnresolved)

	untriaged := readDraft(t, layout)
	writeDraft(t, layout, deleteBlock(t, untriaged, "f1"))
	before := stateFiles(t, layout)

	_, err = runDraft(t, draftPR, "--repo", draftSlug)

	var refused *UnresolvedPostError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, UnresolvedPostError{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum, Round: draftRound,
	}, *refused)
	assert.Equal(t,
		"cr draft moves the round's records, and a send whose outcome cr never learned "+
			"may already have posted them: "+refused.Error(),
		err.Error())
	assert.Equal(t, ExitState, exitCodeFor(err))
	assert.Equal(t,
		"run `cr post <pr> --reconcile` to adopt the review the earlier call created, "+
			"or to clear post_unresolved for a retry",
		hintFor(err))
	assert.Equal(t, before, stateFiles(t, layout),
		"the refusal reads no draft and writes nothing: no state, no waiver, no journal line")
	assert.Equal(t, finding.StateQueued, roundRecordsByID(t, layout)["f1"].State)

	printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
	require.NoError(t, err)
	report := reconcileReport(t, printed)
	assert.Equal(t, adoptedReviewURL, report.Adopted)
	assert.Equal(t, []string{"f1", "f2"}, report.Records)
	assert.False(t, report.Unresolved)
	stored := roundRecordsByID(t, layout)
	assert.Equal(t, finding.StatePosted, stored["f1"].State)
	assert.Equal(t, finding.StatePosted, stored["f2"].State)
	assert.Len(t, shim.writes(t), 1, "§8.4.4: the reconciliation reads and never sends")

	writeDraft(t, layout, untriaged)
	_, err = runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err, "a settled posting no longer refuses the draft")
}
