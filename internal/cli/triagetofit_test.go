package cli

import (
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// otherPR is a second pull request of recordedHome's repository, reviewed at
// the same head over the same two units, so a record raised on it has the same
// waiver key as one raised on recordPR.
const otherPR = recordPRNum + 1

// briefOtherPR opens a round on otherPR the way `cr brief` leaves one.
func briefOtherPR(t *testing.T, layout state.Layout) {
	t.Helper()
	require.NoError(t, layout.EnsurePR(recordOwner, recordRepo, otherPR))
	held, err := layout.LockPR(recordOwner, recordRepo, otherPR)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: recordOwner, Repo: recordRepo, PR: otherPR, Round: 1, Head: recordHead,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(
		unitLine("u1", recordHead, 1, recordUnitStart["u1"])+"\n"+
			unitLine("u2", recordHead, 1, recordUnitStart["u2"])+"\n")))
	require.NoError(t, held.Unlock())
}

// §1.6.3: triaging to fit under the cap is not destructive.
//
// recordPR's round queues two comments against a post.max_comments of 1, and the
// draft header says it is one over. The reviewer deletes f1's block to fit —
// the verb §7.2 gives a true finding not worth saying here — and `cr draft`
// discards it `not-here`, which §7.4.1 scopes to this pull request.
//
// Two merges then carry the same record. On recordPR it is dropped, which is
// the control: the key the discard waived is the key the merge computes, so
// the next assertion is not a key that never matched. On otherPR, in the same
// repository, nothing silences it, and `cr record` stores it in draft.
func TestADiscardForVolumeOnOnePullRequestStillRaisesOnAnother(t *testing.T) {
	layout := recordedHome(t)
	require.NoError(t, layout.EnsureRepo(recordOwner, recordRepo))
	require.NoError(t, os.WriteFile(layout.RepoConfig(recordOwner, recordRepo),
		[]byte(`{"post": {"max_comments": 1}}`), 0o600))
	_, err := runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1"), aRecord("f2", "u2")),
		"--repo", recordSlug)
	require.NoError(t, err)

	draftFile := layout.RoundFile(recordOwner, recordRepo, recordPRNum, recordRound, state.FileDraft)
	require.NoError(t, runCLI(t, "draft", recordPR, "--repo", recordSlug))
	drafted, err := os.ReadFile(draftFile)
	require.NoError(t, err)
	require.Contains(t, string(drafted), "2 comments queued against post.max_comments 1, 1 over the cap")
	require.NoError(t, os.WriteFile(draftFile, []byte(deleteBlock(t, string(drafted), "f1")), 0o600))
	require.NoError(t, runCLI(t, "draft", recordPR, "--repo", recordSlug))
	fitted, err := os.ReadFile(draftFile)
	require.NoError(t, err)
	require.Contains(t, string(fitted), "1 comments queued against post.max_comments 1\n", "the round now fits")

	here, err := finding.ActiveWaivers(layout, recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.Len(t, here, 1)
	assert.Equal(t, finding.DispositionNotHere, here[0].Disposition, "§7.2: a deleted block is not-here")
	scope, err := here[0].Scope()
	require.NoError(t, err)
	assert.Equal(t, finding.ScopePullRequest, scope, "§7.4.1 scopes not-here to the pull request")

	// f3, not f1: §6.1 holds recordPR's f1 and f2 for the life of that pull
	// request, so the merge on it refuses a record reusing either.
	fanOut := writeFanOut(t, t.TempDir(), "correctness", aRoleRecord("f3", "correctness", "unchecked-error", "u1"))
	control, err := runMergeCLI(t, mergedOut(t), fanOut)
	require.NoError(t, err)
	require.Equal(t, 1, decodeMergeResult(t, control).Waived.Dropped,
		"the control: on the pull request that set it aside, the finding stays silenced")

	briefOtherPR(t, layout)
	elsewhere, err := finding.ActiveWaivers(layout, recordOwner, recordRepo, otherPR)
	require.NoError(t, err)
	assert.Empty(t, elsewhere, "§1.6.3: the volume discard silences nothing repository-wide")
	out := mergedOut(t)
	raised, err := runCLIPrinting(t, "merge", fanOut, "-o", out,
		"--repo", recordSlug, "--pr", strconv.Itoa(otherPR))
	require.NoError(t, err)
	reported := decodeMergeResult(t, raised)
	assert.Equal(t, 0, reported.Waived.Dropped, "nothing drops the finding on the other pull request")
	assert.Equal(t, 1, reported.Merged, "the finding set aside for volume still raises there")

	_, err = runRecord(t, strconv.Itoa(otherPR), out, "--repo", recordSlug)
	require.NoError(t, err)
	stored, err := state.ReadRecords[finding.Finding](layout, recordOwner, recordRepo, otherPR, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, "f3", stored[0].ID)
	assert.Equal(t, finding.StateDraft, stored[0].State, "recorded on the other pull request, not dropped")
	assert.Equal(t, recordedHash(t, "u1"), stored[0].Anchor.ContentHash, "under the key the discard waived")
}
