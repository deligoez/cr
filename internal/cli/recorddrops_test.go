package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// dropsHome is recordedHome with a waiver on u1's unchecked-error finding and a
// posted-index entry for u2's, both keyed on the hash cr stamps from the head —
// the hash a waiver or posted entry `cr record` wrote actually carries.
func dropsHome(t *testing.T) state.Layout {
	t.Helper()
	layout := recordedHome(t)
	_, err := finding.Waive(layout, recordOwner, recordRepo, &finding.Waiver{
		WaiverKey: finding.WaiverKey{
			Path: recordPath, Side: "RIGHT", Class: "unchecked-error", ContentHash: recordedHash(t, "u1"),
		},
		Disposition: finding.DispositionNotHere,
	}, finding.WaiverProvenance{Round: recordRound, PR: recordPRNum, Head: recordHead})
	require.NoError(t, err)
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, finding.AppendPosted(held, layout, recordOwner, recordRepo, recordPRNum,
		[]finding.PostedEntry{{
			Record: "f9",
			WaiverKey: finding.WaiverKey{
				Path: recordPath, Side: "RIGHT", Class: "unchecked-error", ContentHash: recordedHash(t, "u2"),
			},
			Round: recordRound - 1, Head: recordHead,
		}}))
	require.NoError(t, held.Unlock())
	return layout
}

// roleRecords are one role's raw findings: a fresh copy of the waived finding,
// a fresh copy of the posted one, and one neither covers. Every anchor carries
// aRecord's typed `0123456789abcdef`, which is not the hash of any line.
func roleRecords() []map[string]any {
	kept := aRecord("f3", "u1")
	kept["class"] = "missing-test"
	return []map[string]any{aRecord("f1", "u1"), aRecord("f2", "u2"), kept}
}

// recordedFrom runs `cr record` over one file and returns what it reported and
// the ids findings.ndjson holds afterwards.
func recordedFrom(t *testing.T, layout state.Layout, file string) (reported recordResult, stored []string) {
	t.Helper()
	printed, err := runCLIPrinting(t, "record", recordPR, file, "--repo", recordSlug)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(printed), &reported))
	held, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	for i := range held {
		stored = append(stored, held[i].ID)
	}
	return reported, stored
}

// §6.4.4 and §9.3.6 at `cr record`: a raw single-role file handed straight to
// the command, with no `cr merge` before it, still loses the waived and the
// already-posted finding, and nothing of either reaches findings.ndjson. The
// invariant is that a posted finding never reaches a second draft, not that a
// particular command ran.
func TestARawRoleFileRecordedWithoutMergeStillDropsWaivedAndPostedFindings(t *testing.T) {
	layout := dropsHome(t)
	raw := writeFanOut(t, t.TempDir(), "correctness", roleRecords()...)

	reported, stored := recordedFrom(t, layout, raw)

	assert.Equal(t, []string{"f3"}, stored, "only the finding neither drop covers is stored")
	assert.Equal(t, 1, reported.Waived.Dropped)
	assert.Equal(t, 1, reported.AlreadyPosted.Dropped)
	assert.Equal(t, []string{"f9"}, reported.AlreadyPosted.Posted)
	assert.Contains(t, reported.Honesty, reported.Waived.Disclosure())
	assert.Contains(t, reported.Honesty, reported.AlreadyPosted.Disclosure())
}

// Both commands match on a hash cr computed from the head: the fresh copies
// carry a wrong typed hash, and they are dropped by `cr merge` and, handed to
// `cr record` directly, dropped again. Before merge stamped the anchor it
// matched the typed hash and dropped neither.
func TestAWrongTypedHashIsDroppedAtMergeAndAgainAtRecord(t *testing.T) {
	layout := dropsHome(t)
	raw := writeFanOut(t, t.TempDir(), "correctness", roleRecords()...)

	out := mergedOut(t)
	printed, err := runMergeCLI(t, out, raw)
	require.NoError(t, err)
	merged := decodeMergeResult(t, printed)
	assert.Equal(t, 1, merged.Merged, "§6.4.4 and §9.3.6 at merge, on the stamped hash")
	assert.Equal(t, 1, merged.Waived.Dropped)
	assert.Equal(t, 1, merged.AlreadyPosted.Dropped)

	reported, stored := recordedFrom(t, layout, raw)
	assert.Equal(t, []string{"f3"}, stored)
	assert.Equal(t, 1, reported.Waived.Dropped, "§6.4.4 again at record")
	assert.Equal(t, 1, reported.AlreadyPosted.Dropped, "§9.3.6 again at record")
}

// Over `cr merge`'s own output the second pass takes nothing out, so recording
// it drops nothing and discloses no drop.
func TestRecordingMergesOwnOutputDropsNothingTwice(t *testing.T) {
	layout := dropsHome(t)
	out := mergedOut(t)
	_, err := runMergeCLI(t, out, writeFanOut(t, t.TempDir(), "correctness", roleRecords()...))
	require.NoError(t, err)

	reported, stored := recordedFrom(t, layout, out)

	assert.Equal(t, []string{"f3"}, stored)
	assert.Zero(t, reported.Waived.Dropped)
	assert.Zero(t, reported.AlreadyPosted.Dropped)
	assert.NotContains(t, reported.Honesty, reported.Waived.Disclosure())
}

// A group whose representative the file does not carry and the round has not
// stored is re-elected among the records that remain, per §6.4.2, rather than
// left with every member in `draft`.
//
// gremlins found the re-election unasserted: skipping it when there were
// orphans left both records of one anchored line and class in `draft`, so the
// round would draft the same finding twice — which §6.4.3's suppression exists
// to prevent. f2 and f3 sit on one anchored line in one class and both name f1,
// which no file handed in and no earlier run stored.
func TestAGroupWhoseRepresentativeIsAbsentIsReElectedAmongTheRest(t *testing.T) {
	layout := recordedHome(t)
	orphan := func(id string) map[string]any {
		record := aRecord(id, "u1")
		record["duplicate_of"] = "f1"
		return record
	}
	file := asMergeOutput(t, layout, writeRecordFile(t, "merged.ndjson", orphan("f2"), orphan("f3")))

	_, err := runRecord(t, recordPR, file, "--repo", recordSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	states := []finding.State{stored[0].State, stored[1].State}
	assert.ElementsMatch(t, []finding.State{finding.StateDraft, finding.StateDuplicate}, states,
		"§6.4.2: one of the two speaks for the group and the other is retired for it")
	for i := range stored {
		assert.NotEqual(t, "f1", stored[i].DuplicateOf, "no stored record names a representative that is not there")
	}
}
