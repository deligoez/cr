package cli

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// keyTreesFailing is standInKeyTrees' tree for §7.4.1's key, whose read of a
// path fails as a git that cannot read the head fails whenever fails answers
// true for it.
func keyTreesFailing(t *testing.T, fails func(path string) bool) {
	t.Helper()
	restore := keyTrees
	keyTrees = func(string, string, int, string) finding.Trees {
		read := func(path string) ([]string, bool, error) {
			if fails(path) {
				return nil, false, &git.CommandError{
					Args:   []string{"ls-tree", draftHead, "--", path},
					Stderr: "fatal: not a tree object",
					Err:    errors.New("exit status 128"),
				}
			}
			lines := make([]string, 0, standInLines)
			for n := 1; n <= standInLines; n++ {
				lines = append(lines, fmt.Sprintf("%s line %d", path, n))
			}
			return lines, true, nil
		}
		return finding.Trees{Head: read, MergeBase: read}
	}
	t.Cleanup(func() { keyTrees = restore })
}

// aPostWithADiscard is a drafted round whose draft keeps f1 and deletes f2's
// block, each anchored in a file of its own, and a gh shim answering the
// confirmed send of what is left.
func aPostWithADiscard(t *testing.T) (
	layout state.Layout, shim *ghShimTranscript, kept, discarded *finding.Finding,
) {
	t.Helper()
	kept, discarded = aCitedRecord("f1"), aCitedRecord("f2")
	discarded.Anchor.Path = "internal/api/discarded.go"
	layout = draftedHome(t, kept, discarded)
	redraft(t)
	require.NoError(t, os.WriteFile(
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft),
		[]byte(deleteBlock(t, readDraft(t, layout), "f2")), 0o600))
	return layout, ghShimming(t, builtPayload(t)), kept, discarded
}

// §8.4.4 over the keys: `cr post --confirm` reads every tree §7.4.1's waiver
// keys and §9.3.6's index keys need before it writes or sends anything, so a
// read that fails refuses the post with nothing sent and nothing written, and
// the round is postable again once the tree reads.
//
// One row fails the discard's waiver key and the other the posted record's
// index key, since a send forms both. The state root is compared file by file,
// so posted.json, findings.ndjson, meta.json's `post_unresolved` and both
// waiver files are all held to what they were.
func TestAKeyTreeThatCannotBeReadRefusesThePostBeforeAnythingIsSent(t *testing.T) {
	for _, row := range []struct {
		name string
		path func(kept, discarded *finding.Finding) string
	}{
		{"the discard's waiver key", func(_, discarded *finding.Finding) string { return discarded.Anchor.Path }},
		{"the posted record's index key", func(kept, _ *finding.Finding) string { return kept.Anchor.Path }},
	} {
		t.Run(row.name, func(t *testing.T) {
			layout, shim, kept, discarded := aPostWithADiscard(t)
			unreadable, readable := row.path(kept, discarded), false
			keyTreesFailing(t, func(path string) bool { return !readable && path == unreadable })
			before := stateFiles(t, layout)

			_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")

			require.Error(t, err)
			var failed *git.CommandError
			require.ErrorAs(t, err, &failed, "the refusal carries the read that failed")
			assert.Equal(t, ExitFile, exitCodeFor(err), "§11.2 codes a failed external read 3")
			assert.Equal(t, "the git command the message names failed; run it yourself to see what it reports",
				hintFor(err))
			assert.Empty(t, shim.writes(t), "§8.4.4: nothing reached GitHub")
			assert.Equal(t, before, stateFiles(t, layout),
				"no posted.json, no post_unresolved, and findings and waivers as they were")

			readable = true
			_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")

			require.NoError(t, err, "the refused round is still postable")
			assert.Len(t, shim.writes(t), 1)
			assert.Equal(t, "pull request", waiverScopeOf(t, layout, discarded.Anchor.Path))
			assert.Equal(t, []string{"f1"}, entryRecords(postedIndexOf(t, layout)))
		})
	}
}

// The other half: once the call has returned, `cr post --confirm` stores the
// keys it formed before it and reads no tree for them. A tree that stops
// reading the moment the shim has logged the POST leaves the run whole — the
// records posted, the discard's waiver and the posted record's index entry
// written under the review the call created, and `post_unresolved` cleared —
// and no key read reaches the tree after the call.
func TestAKeyTreeThatStopsReadingAfterTheRequestLeavesThePostWhole(t *testing.T) {
	layout, shim, _, discarded := aPostWithADiscard(t)
	lateReads := 0
	keyTreesFailing(t, func(string) bool {
		if len(shim.writes(t)) > 0 {
			lateReads++
			return true
		}
		return false
	})

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")

	require.NoError(t, err)
	require.Len(t, shim.writes(t), 1)
	assert.Zero(t, lateReads, "no key read reaches a tree after the request")
	stored, err := state.ReadStamped[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings, draftRound)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	assert.Equal(t, []finding.State{finding.StatePosted, finding.StateDiscarded},
		[]finding.State{stored[0].State, stored[1].State})
	assert.Equal(t, "pull request", waiverScopeOf(t, layout, discarded.Anchor.Path))
	index := postedIndexOf(t, layout)
	assert.Equal(t, []string{"f1"}, entryRecords(index))
	require.Len(t, index, 1)
	assert.Equal(t, createdReviewID, index[0].Review, "the entry names the review the call created")
	meta, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	assert.False(t, meta.PostUnresolved)
}

// entryRecords are the record ids a posted index holds, in its order.
func entryRecords(index []finding.PostedEntry) []string {
	ids := make([]string, 0, len(index))
	for i := range index {
		ids = append(ids, index[i].Record)
	}
	return ids
}
