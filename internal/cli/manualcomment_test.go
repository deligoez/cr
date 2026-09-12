package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// handWritten is a block a reviewer typed into the draft themselves: a marker
// of the right grammar naming a record the round does not hold, and prose of
// their own beneath it.
//
// It is well-formed on purpose. §7.2.1 already refuses a marker that deviates
// from §7.1.1's grammar, and a malformed one would be refused for that instead
// — which would leave §7.2.3's own obligation untested while looking tested.
const handWritten = "\n" +
	`<!-- cr:record id="f9" kind="finding" path="internal/api/handler.go" ` +
	`start_line="42" line="44" severity="high" grade="cited" disposition="" -->` +
	"\n\nI would rather say this one in my own name.\n"

// runsReadingTheDraft are the two commands §7.2 has interpret the reviewer's
// file, each as it is invoked.
var runsReadingTheDraft = map[string]func(*testing.T, ...string) (string, error){
	"cr draft": runDraft,
	"cr post":  runPost,
}

// §7.2.3: a block whose id is unknown aborts with exit code 1 rather than being
// adopted as a new record.
//
// v0.1 has no manual-comment channel in the draft, and the reason is in the
// corpus rather than in the parser: every record carries a role, an axis and a
// grade cr computed, and a hand-written block can carry none of them — so a
// block adopted here would be a comment with no producer, on no axis, resting
// on nothing, and §6.2's grade would have nothing to compute from. The refusal
// says so and names where the comment does belong: GitHub, in the reviewer's
// own name, after the review is posted.
//
// Both commands that read the draft are asked, because §7.2 gives the file two
// readers and a refusal in one of them is a channel in the other.
func TestAHandWrittenBlockAbortsNamingTheUnknownID(t *testing.T) {
	for name, run := range runsReadingTheDraft {
		t.Run(name, func(t *testing.T) {
			layout := draftedHome(t, aCitedRecord("f1"))
			redraft(t)
			writeDraft(t, layout, readDraft(t, layout)+handWritten)

			_, err := run(t, draftPR, "--repo", draftSlug)

			require.Error(t, err)
			assert.Equal(t, ExitValidation, exitCodeFor(err), "§7.2.3 aborts with exit code 1")
			assert.Contains(t, err.Error(), "f9", "naming the id the round does not hold")
			assert.Contains(t, err.Error(), "no manual-comment channel",
				"and saying why, since the reviewer's next move is to write it on GitHub")
			assert.Equal(t, finding.StateQueued, draftedFindings(t, layout)[0].State,
				"the round's own record is where it was: nothing was adopted and nothing moved")
		})
	}
}

// The refusal is made before any of §7.2's verbs are read, so a draft carrying
// a hand-written block and a deletion loses neither.
//
// A reviewer who pasted a block in has almost certainly triaged the rest of the
// file in the same sitting. If the deletion were acted on first, the run would
// report the typo and discard f1 behind a waiver in the same breath — and the
// reviewer would have to undo a waiver they never asked for before they could
// fix the block they did.
func TestAHandWrittenBlockIsRefusedBeforeAnyVerbIsRead(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
	redraft(t)
	writeDraft(t, layout, deleteBlock(t, readDraft(t, layout), "f2")+handWritten)

	_, err := runDraft(t, draftPR, "--repo", draftSlug)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "f9")
	for _, record := range draftedFindings(t, layout) {
		assert.Equal(t, finding.StateQueued, record.State,
			"%s: the deletion is still the reviewer's to confirm on the next run", record.ID)
	}
	waivers, err := finding.PullRequestWaivers(layout, draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	assert.Empty(t, waivers, "§7.2's waiver is written by the run that acts on the deletion")
}
