package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// markerLineOf is the one-based line of id's marker in a draft.
func markerLineOf(t *testing.T, file, id string) int {
	t.Helper()
	for i, line := range strings.Split(file, "\n") {
		if strings.HasPrefix(line, `<!-- cr:record id="`+id+`"`) {
			return i + 1
		}
	}
	require.Failf(t, "no marker", "the draft holds no block for %s", id)
	return 0
}

// twoRecordDraft drafts two records at different locations, f2 above f1, and
// returns the rendered draft with f2's marker id changed to f1 and the lines
// both markers sat on.
func twoRecordDraft(t *testing.T) (layout state.Layout, edited string, f2At, f1At int) {
	t.Helper()
	moved := aStoredRecord("f2", finding.StateDraft)
	moved.Anchor.Path = "internal/api/money.go"
	moved.Severity = finding.SeverityMedium
	layout = draftedHome(t, moved, aStoredRecord("f1", finding.StateDraft))
	redraft(t)
	rendered := readDraft(t, layout)
	f2At, f1At = markerLineOf(t, rendered, "f2"), markerLineOf(t, rendered, "f1")
	require.Less(t, f2At, f1At, "the edited marker is the earlier one, as in QA D-S08-4")
	return layout, markerEdit(t, rendered, "f2", `id="f2"`, `id="f1"`), f2At, f1At
}

// assertIDEditRefused checks both commands that read the draft refuse it as
// §7.2's changed id, naming the edited line, the id it repeats and the id to
// restore, and that nothing was triaged.
func assertIDEditRefused(t *testing.T, layout state.Layout, want *draft.MarkerIDEditError) {
	t.Helper()
	for _, run := range []func(*testing.T, ...string) (string, error){runDraft, runPost} {
		_, err := run(t, draftPR, "--repo", draftSlug)
		var refused *draft.MarkerIDEditError
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, want, refused)
		assert.Equal(t, ExitValidation, exitCodeFor(err), "§7.2 codes it 1")
		assert.Contains(t, hintFor(err), "put back the id cr wrote")
		assert.NotContains(t, hintFor(err), "run `cr draft` again",
			"a second run reads the same file, so the hint does not offer one")
	}
	stored := draftedFindings(t, layout)
	for i := range stored {
		assert.Equal(t, finding.StateQueued, stored[i].State, "nothing was triaged: %s", stored[i].ID)
	}
}

// §7.2's immutable `id` through the command: a marker whose id was changed to
// another record's is refused naming the edited line, and deleting the other
// record's block afterwards is refused too, rather than handing the edited
// block to that record.
//
// QA D-S08-4: f701's marker at line 11 was changed to f1, and cr named line 21
// — f1's untouched marker — as malformed, with a hint to run `cr draft` again.
// Deleting the block it named was accepted: f701 was discarded not-here with a
// waiver, and f1 was softened, re-anchored onto f701's line and re-severitied,
// carrying f701's prose.
func TestAMarkerIDChangedToAnotherRecordsIsRefusedAtTheEditedLine(t *testing.T) {
	t.Run("both blocks present", func(t *testing.T) {
		layout, edited, f2At, f1At := twoRecordDraft(t)
		writeDraft(t, layout, edited)

		assertIDEditRefused(t, layout, &draft.MarkerIDEditError{At: f2At, ID: "f1", Kept: f1At, Restore: "f2"})
	})

	t.Run("the other record's block deleted", func(t *testing.T) {
		layout, edited, f2At, _ := twoRecordDraft(t)
		writeDraft(t, layout, onlyBlockOf(t, edited, f2At))

		assertIDEditRefused(t, layout, &draft.MarkerIDEditError{At: f2At, ID: "f1", Restore: "f2"})
	})

	t.Run("the other record's block deleted and the severity edited too", func(t *testing.T) {
		layout, edited, f2At, _ := twoRecordDraft(t)
		reworded := strings.Replace(onlyBlockOf(t, edited, f2At), `severity="medium"`, `severity="low"`, 1)
		writeDraft(t, layout, reworded)

		assertIDEditRefused(t, layout, &draft.MarkerIDEditError{At: f2At, ID: "f1", Restore: "f2"})
	})

	t.Run("the other record's block deleted and the body reworded too", func(t *testing.T) {
		layout, edited, f2At, _ := twoRecordDraft(t)
		reworded := strings.Replace(onlyBlockOf(t, edited, f2At), "dropped, f2.", "lost on this path.", 1)
		require.NotEqual(t, onlyBlockOf(t, edited, f2At), reworded)
		writeDraft(t, layout, reworded)

		assertIDEditRefused(t, layout, &draft.MarkerIDEditError{At: f2At, ID: "f1", Restore: "f2"})
	})

	t.Run("two records at one location with one body, one deleted", func(t *testing.T) {
		twin := aStoredRecord("f2", finding.StateDraft)
		twin.Class = "swallowed-error"
		first := aStoredRecord("f1", finding.StateDraft)
		twin.Summary = first.Summary
		layout := draftedHome(t, first, twin)
		redraft(t)
		writeDraft(t, layout, deleteBlock(t, readDraft(t, layout), "f2"))

		assert.Equal(t, regenerated{
			Triaged:   []triagedRecord{{ID: "f2", Outcome: finding.OutcomeDiscardedNotHere}},
			Preserved: []string{},
		}, redraft(t), "f1's untouched block reads f2's location and body only because they are f1's too")
	})

	t.Run("a deletion beside an edit of the kept block is still a deletion", func(t *testing.T) {
		layout, _, _, _ := twoRecordDraft(t)
		rendered := readDraft(t, layout)
		kept := markerEdit(t, deleteBlock(t, rendered, "f2"), "f1", `severity="high"`, `severity="medium"`)
		writeDraft(t, layout, kept)

		assert.Equal(t, regenerated{
			Triaged:   []triagedRecord{{ID: "f2", Outcome: finding.OutcomeDiscardedNotHere}},
			Preserved: []string{},
		}, redraft(t))
	})
}

// onlyBlockOf is a draft whose one block is the block whose marker sits at
// line at, with the header kept: the draft a reviewer leaves after deleting
// every other block.
func onlyBlockOf(t *testing.T, file string, at int) string {
	t.Helper()
	lines := strings.Split(file, "\n")
	first := 0
	for i, line := range lines {
		if strings.HasPrefix(line, "<!-- cr:record ") {
			first = i
			break
		}
	}
	end := len(lines)
	for i := at; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "<!-- cr:record ") {
			end = i
			break
		}
	}
	kept := append(append([]string{}, lines[:first]...), lines[at-1:end]...)
	return strings.Join(kept, "\n")
}
