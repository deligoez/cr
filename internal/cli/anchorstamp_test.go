package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// storedFindings reads recordedHome's findings.ndjson whole.
func recordedFindings(t *testing.T, layout state.Layout) []finding.Finding {
	t.Helper()
	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	return stored
}

// recordedHash is the §9.2 content hash cr stamps on aRecord's anchor in one
// unit: recordPath's lines two to four into the unit's range, at recordHead.
// A waiver or posted-index fixture keyed on anything else matches no record cr
// stamped.
func recordedHash(t *testing.T, unit string) string {
	t.Helper()
	start := recordUnitStart[unit]
	hash, err := finding.AnchorContentHash(handlerLines(start+2, start+4))
	require.NoError(t, err)
	return hash
}

// recordedKeyHash is §7.4.1's key hash of aRecord's anchor in one unit, which a
// waiver or posted-index fixture is keyed on: the three lines above recordedHash's
// lines, those lines, and the three below, at recordHead.
func recordedKeyHash(t *testing.T, unit string) string {
	t.Helper()
	start := recordUnitStart[unit]
	hash, err := finding.ContextKeyHash(
		handlerLines(start-1, start+1), handlerLines(start+2, start+4), handlerLines(start+5, start+7))
	require.NoError(t, err)
	return hash
}

// handlerLines is recordCheckout's file between two line numbers, inclusive.
func handlerLines(from, to int) []string {
	lines := make([]string, 0, to-from+1)
	for n := from; n <= to; n++ {
		lines = append(lines, fmt.Sprintf("handler line %d", n))
	}
	return lines
}

// §9.2.3 through `cr record`: a stored anchor carries the content hash of its
// own lines at the round's head and three lines of context on each side, not
// whatever the agent wrote, and the waiver §7.4.1 writes from a discard of that
// record carries the same hash.
//
// aRecord supplies `content_hash: "0123456789abcdef"`, which hashes nothing the
// tree holds. Measured before the stamp at 8cdf48b: that value reached
// findings.ndjson unchanged, and on 2026-09-11 a real pull request's anchors all
// arrived with an empty one.
func TestARecordedAnchorCarriesTheHashOfItsLinesAndItsWaiverTheSame(t *testing.T) {
	layout := recordedHome(t)
	_, err := runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1")),
		"--repo", recordSlug)
	require.NoError(t, err)

	stored := recordedFindings(t, layout)
	require.Len(t, stored, 1)
	anchor := stored[0].Anchor
	want, err := finding.AnchorContentHash(handlerLines(42, 44))
	require.NoError(t, err)
	assert.Equal(t, want, anchor.ContentHash, "§9.2: the hash of lines 42 to 44 at the round's head")
	assert.Equal(t, handlerLines(39, 41), anchor.ContextBefore, "§9.2: three lines above the range")
	assert.Equal(t, handlerLines(45, 47), anchor.ContextAfter, "§9.2: three lines below it")

	_, err = runDraft(t, recordPR, "--repo", recordSlug)
	require.NoError(t, err)
	draft := layout.RoundFile(recordOwner, recordRepo, recordPRNum, recordRound, state.FileDraft)
	written, err := os.ReadFile(draft)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(draft, []byte(deleteBlock(t, string(written), "f1")), 0o600))
	_, err = runDraft(t, recordPR, "--repo", recordSlug)
	require.NoError(t, err)

	waivers, err := finding.PullRequestWaivers(layout, recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.Len(t, waivers, 1, "§7.2: a deleted block writes a pull-request waiver")
	assert.Equal(t, recordedKeyHash(t, "u1"), waivers[0].ContentHash,
		"§7.4.1: the waiver is keyed on the anchored lines with their context window")
	assert.NotEqual(t, want, waivers[0].ContentHash, "§9.2's anchor hash stays over the anchored lines alone")
}

// §9.2.2 through `cr brief`: a head change moves the round's open records to
// `stale` and migrates no anchor. The head below inserts five lines above the
// anchored ones, so a migration would have to move the range to 47 to 49; the
// stored anchor keeps 42 to 44, its hash, and its context window on disk.
func TestAHeadChangeStalesTheRecordsAndMigratesNoAnchor(t *testing.T) {
	layout := recordedHome(t)
	_, err := runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1")),
		"--repo", recordSlug)
	require.NoError(t, err)
	recorded := recordedFindings(t, layout)[0].Anchor

	dir, err := repoDir()
	require.NoError(t, err)
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	inserted := append(handlerLines(-4, 0), handlerLines(1, 100)...)
	require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(recordPath)),
		[]byte(strings.Join(inserted, "\n")+"\n"), 0o600))
	mustGit(t, dir, "commit", "--quiet", "-am", "five lines above the anchored ones")
	moved := strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))
	t.Setenv("PATH", ghShim(t, t.TempDir(), moved, recordHead)+string(os.PathListSeparator)+os.Getenv("PATH"))
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte("CR-13: the handler retries.\n"), 0o600))

	_, err = runCLIPrinting(t, "brief", recordPR, "--repo", recordSlug, "--issue", "CR-13", "--intent-file", issue)
	require.NoError(t, err)

	after := recordedFindings(t, layout)
	require.Len(t, after, 1)
	assert.Equal(t, finding.StateStale, after[0].State, "§9.3.4: an open record moves to stale on the increment")
	assert.Equal(t, recorded, after[0].Anchor, "§9.2.2: the anchor is bound to the head it was produced against")
	assert.Equal(t, 42, after[0].Anchor.StartLine, "no migration to the lines the content moved to")
	assert.Equal(t, handlerLines(39, 41), after[0].Anchor.ContextBefore,
		"§9.2.3: the context window survives on disk for a v0.3 migration")
}
