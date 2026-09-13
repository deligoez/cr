package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// groupedRecords are two correctness findings §6.4.1 groups — one class, one
// path, one anchored last line 44 — merged by `cr merge` into f1 as the
// representative (severity high over medium) and f2 as its duplicate. f2's
// anchor starts one line lower unless sameLines, so its §9.2 content hash, and
// with it its §7.4.1 waiver key, differs from f1's.
func groupedRecords(t *testing.T, sameLines bool) string {
	t.Helper()
	representative := aRecord("f1", "u1")
	duplicate := aRecord("f2", "u1")
	duplicate["severity"] = "medium"
	if !sameLines {
		duplicate["anchor"] = map[string]any{
			"path": recordPath, "side": "RIGHT", "start_line": 43, "line": 44,
		}
	}
	out := mergedOut(t)
	printed, err := runMergeCLI(t, out, writeFanOut(t, t.TempDir(), "correctness", representative, duplicate))
	require.NoError(t, err)
	require.Equal(t, 2, decodeMergeResult(t, printed).Merged, "the merge keeps both, f2 marked a duplicate")
	return out
}

// waiveTheRepresentative waives f1's key: unchecked-error at recordPath 42..44.
func waiveTheRepresentative(t *testing.T, layout state.Layout) {
	t.Helper()
	_, err := finding.Waive(layout, recordOwner, recordRepo, &finding.Waiver{
		WaiverKey: finding.WaiverKey{
			Path: recordPath, Side: "RIGHT", Class: "unchecked-error", ContentHash: recordedHash(t, "u1"),
		},
		Disposition: finding.DispositionNotHere,
	}, finding.WaiverProvenance{Round: recordRound, PR: recordPRNum, Head: recordHead})
	require.NoError(t, err)
}

// A waiver added between `cr merge` and `cr record` that matches only the
// group's representative leaves the other member stored in draft, re-elected as
// its group's representative with no duplicate_of, rather than retired as a
// duplicate of a record the round never holds.
func TestADroppedRepresentativeLeavesItsDuplicateStoredInDraft(t *testing.T) {
	layout := recordedHome(t)
	merged := groupedRecords(t, false)
	waiveTheRepresentative(t, layout)

	reported, stored := recordedFrom(t, layout, merged)

	assert.Equal(t, 1, reported.Waived.Dropped)
	require.Equal(t, []string{"f2"}, stored)
	held, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	assert.Empty(t, held[0].DuplicateOf, "no stored record names a representative the round does not hold")
	assert.Equal(t, finding.StateDraft, held[0].State)
}

// A group whose every member the waiver covers stores nothing of the group.
func TestAGroupWhoseEveryMemberIsWaivedStoresNothing(t *testing.T) {
	layout := recordedHome(t)
	merged := groupedRecords(t, true)
	waiveTheRepresentative(t, layout)
	findings := layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings)

	reported, stored := recordedFrom(t, layout, merged)

	assert.Equal(t, 2, reported.Waived.Dropped)
	assert.Empty(t, stored)
	body, err := os.ReadFile(findings)
	require.NoError(t, err)
	assert.Empty(t, body)
}
