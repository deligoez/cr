package cli

import (
	"encoding/json"
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

// standInLines is how many lines every file of standInKeyTrees' tree holds.
const standInLines = 200

// standInKeyTrees stands a tree in for §7.4.1's key, for a fixture round whose
// head is no commit: every path holds standInLines lines, each naming its path
// and its number, in the head and the merge base alike.
func standInKeyTrees(t *testing.T) {
	t.Helper()
	restore := keyTrees
	keyTrees = func(string, string, int, string) finding.Trees {
		read := func(path string) ([]string, bool, error) {
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

// treesOf are the trees §7.4.1's key reads a fixture record's anchored lines
// from, at the head it was produced against.
func treesOf(record *finding.Finding) finding.Trees {
	return keyTrees(draftOwner, draftRepo, draftPRNum, record.Head)
}

// keyOf is §7.4.1's key of a fixture record, formed as cr forms it.
func keyOf(t *testing.T, record *finding.Finding) finding.WaiverKey {
	t.Helper()
	key, err := finding.WaiverKeyOf(treesOf(record), record)
	require.NoError(t, err)
	return key
}

// openRoundEditing moves recordedHome's pull request on to the round after the
// one it was recorded in, at a head that differs from recordHead in the given
// lines of recordPath alone, with the same two units: the state a `cr brief` on
// that head leaves for the merge the round runs. With no line given the head
// does not move, as openNextRound leaves it.
func openRoundEditing(t *testing.T, layout state.Layout, lines ...int) {
	t.Helper()
	if len(lines) == 0 {
		openNextRound(t, layout)
		return
	}
	dir, err := repoDir()
	require.NoError(t, err)
	body := handlerLines(1, 100)
	for _, line := range lines {
		body[line-1] = fmt.Sprintf("handler line %d, edited", line)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(recordPath)),
		[]byte(strings.Join(body, "\n")+"\n"), 0o600))
	mustGit(t, dir, "commit", "--quiet", "-am", "edit lines around the anchored ones")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))
	next := recordRound + 1
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: recordOwner, Repo: recordRepo, PR: recordPRNum, Round: next, Head: head,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(
		unitLine("u1", head, next, recordUnitStart["u1"])+"\n"+
			unitLine("u2", head, next, recordUnitStart["u2"])+"\n")))
	require.NoError(t, held.Unlock())
}

// §7.4.1 keys a waiver, and §9.3.6 the posted index, by the anchored lines and
// the context window around them, and §7.4.2 has either stop suppressing once
// "that code or its context changes".
//
// Both are written the way cr writes them: the waiver by `cr draft` from a
// deleted block, the posted-index entry by the writer `cr post` appends
// through, over the record `cr record` stored. Then a later round's merge meets
// the same class at the same anchored lines. With nothing edited both still
// drop; with one line of §9.2's three-line window edited, above or below, and
// the anchored lines untouched, neither does; with a line one past the window
// edited, both still do, so the key reaches exactly as far as the window.
//
// Measured with WaiverKeyOf overlaid to key on the anchor's own content hash,
// v0.1's key: the two edited-window cases fail and the other two pass.
func TestAWaiverAndAPostedEntryStopSuppressingOnceTheAnchoredLinesContextChanges(t *testing.T) {
	for name, edit := range map[string]struct {
		// offset is the line edited, counted from each unit's first
		// line; aRecord anchors the unit's lines 2 to 4, so 1 is the line
		// above them and 5 the line below. Zero edits nothing.
		offset     int
		suppressed bool
	}{
		"nothing edited":                    {offset: 0, suppressed: true},
		"the line above the anchored lines": {offset: 1, suppressed: false},
		"the line below the anchored lines": {offset: 5, suppressed: false},
		"a line past the context window":    {offset: 8, suppressed: true},
	} {
		t.Run(name, func(t *testing.T) {
			layout := recordedHome(t)
			_, err := runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson",
				aRecord("f1", "u1"), aRecord("f2", "u2")), "--repo", recordSlug)
			require.NoError(t, err)

			_, err = runDraft(t, recordPR, "--repo", recordSlug)
			require.NoError(t, err)
			draft := layout.RoundFile(recordOwner, recordRepo, recordPRNum, recordRound, state.FileDraft)
			written, err := os.ReadFile(draft)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(draft, []byte(deleteBlock(t, string(written), "f1")), 0o600))
			_, err = runDraft(t, recordPR, "--repo", recordSlug)
			require.NoError(t, err)

			meta, err := layout.ReadMeta(recordOwner, recordRepo, recordPRNum)
			require.NoError(t, err)
			posted := recordOf(asPointers(recordedFindings(t, layout)), "f2")
			require.NotNil(t, posted)
			require.NoError(t, recordPostedIndex(layout, &meta, []*finding.Finding{posted}, createdReviewID))

			lines := make([]int, 0, 2)
			if edit.offset > 0 {
				lines = append(lines, recordUnitStart["u1"]+edit.offset, recordUnitStart["u2"]+edit.offset)
			}
			openRoundEditing(t, layout, lines...)
			printed, err := runMergeCLI(t, mergedOut(t), writeFanOut(t, t.TempDir(), "correctness",
				aRoleRecord("f3", "correctness", "unchecked-error", "u1"),
				aRoleRecord("f4", "correctness", "unchecked-error", "u2")))
			require.NoError(t, err)
			reported := decodeMergeResult(t, printed)

			dropped := 0
			if edit.suppressed {
				dropped = 1
			}
			assert.Equal(t, dropped, reported.Waived.Dropped, "§6.4.4 over the waiver `cr draft` wrote")
			assert.Equal(t, dropped, reported.AlreadyPosted.Dropped, "§9.3.6 over the entry `cr post` appends")
			assert.Equal(t, 2-2*dropped, reported.Merged)
		})
	}
}

// asPointers is records as the slice of pointers recordOf reads.
func asPointers(records []finding.Finding) []*finding.Finding {
	pointers := make([]*finding.Finding, 0, len(records))
	for i := range records {
		pointers = append(pointers, &records[i])
	}
	return pointers
}

// A waiver and a posted-index entry written under v0.1's key, which hashed the
// anchored lines alone, match no key formed now, so nothing migrates them into
// silencing anything; and both of §7.4.4's waivers are still listed by
// `cr waivers list` and removed by `cr waivers remove`, so neither is a one-way
// door. The anchored lines of the unit both are about have a context window on
// each side, so the v0.1 hash and §7.4.1's differ for the records merged here.
func TestAWaiverOrPostedEntryUnderTheV01KeySuppressesNothingAndStaysRemovable(t *testing.T) {
	layout := recordedHome(t)
	legacy := func(unit string) finding.WaiverKey {
		return finding.WaiverKey{
			Path: recordPath, Side: "RIGHT", Class: "unchecked-error", ContentHash: recordedHash(t, unit),
		}
	}
	require.NotEqual(t, recordedHash(t, "u1"), recordedKeyHash(t, "u1"))
	provenance := finding.WaiverProvenance{Round: recordRound, PR: recordPRNum, Head: recordHead}
	wide, err := finding.Waive(layout, recordOwner, recordRepo,
		&finding.Waiver{WaiverKey: legacy("u1"), Disposition: finding.DispositionWrong}, provenance)
	require.NoError(t, err)
	here, err := finding.Waive(layout, recordOwner, recordRepo,
		&finding.Waiver{WaiverKey: legacy("u1"), Disposition: finding.DispositionNotHere}, provenance)
	require.NoError(t, err)
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, finding.AppendPosted(held, layout, recordOwner, recordRepo, recordPRNum,
		[]finding.PostedEntry{{Record: "f9", WaiverKey: legacy("u2"), Round: recordRound - 1, Head: recordHead}}))
	require.NoError(t, held.Unlock())

	printed, err := runMergeCLI(t, mergedOut(t), writeFanOut(t, t.TempDir(), "correctness",
		aRoleRecord("f1", "correctness", "unchecked-error", "u1"),
		aRoleRecord("f2", "correctness", "unchecked-error", "u2")))
	require.NoError(t, err)
	reported := decodeMergeResult(t, printed)
	assert.Equal(t, 0, reported.Waived.Dropped, "a v0.1 waiver silences nothing under §7.4.1's key")
	assert.Equal(t, 0, reported.AlreadyPosted.Dropped, "a v0.1 posted-index entry drops nothing either")
	assert.Equal(t, 2, reported.Merged)

	listed, err := runCLIPrinting(t, "waivers", "list", "--repo", recordSlug, "--pr", recordPR)
	require.NoError(t, err)
	var waivers listedWaivers
	require.NoError(t, json.Unmarshal([]byte(listed), &waivers))
	ids := make([]string, 0, len(waivers.Waivers))
	for _, waiver := range waivers.Waivers {
		ids = append(ids, waiver.ID)
	}
	assert.Equal(t, []string{wide.ID, here.ID}, ids, "§7.4.7 lists both, whatever key they were written under")

	_, err = runCLIPrinting(t, "waivers", "remove", wide.ID, "--repo", recordSlug)
	require.NoError(t, err)
	_, err = runCLIPrinting(t, "waivers", "remove", here.ID, "--repo", recordSlug, "--pr", recordPR)
	require.NoError(t, err)
	active, err := finding.ActiveWaivers(layout, recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	assert.Empty(t, active, "§7.4.4: both are removable, so neither is a one-way door")
}
