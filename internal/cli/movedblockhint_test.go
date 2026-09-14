package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/draft"
)

// The moved-block refusal names the edit the reviewer was reaching for: a
// block that carries a deleted record's location, severity and grade or its
// body under another id is refused, and the hint says to keep the deleted
// record at that location and edit its body there rather than delete it and
// move another record onto its line.
//
// QA D-S08-4's second half: f701's block was deleted after f1's marker had
// been moved onto its line, and the refusal offered only to put the id back.
func TestTheMovedBlockRefusalHintsKeepingTheRecordAndEditingItsBody(t *testing.T) {
	const want = "put back the id cr wrote on the marker line the message names; a block never " +
		"changes record. To say something else at a record's location, keep that record " +
		"there and edit its body instead of deleting it and moving another record onto " +
		"its line. `cr draft` reads the same file again"

	cases := []struct {
		name    string
		edit    func(block string) string
		changes bool
	}{
		{"the deleted record's marker fields carried", func(block string) string { return block }, false},
		{"the deleted record's body carried", func(block string) string {
			return strings.Replace(block, `severity="medium"`, `severity="low"`, 1)
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			layout, edited, f2At, _ := twoRecordDraft(t)
			block := onlyBlockOf(t, edited, f2At)
			require.Equal(t, c.changes, block != c.edit(block), "the case edits what it names")
			writeDraft(t, layout, c.edit(block))

			for _, run := range []func(*testing.T, ...string) (string, error){runDraft, runPost} {
				_, err := run(t, draftPR, "--repo", draftSlug)
				var refused *draft.MarkerIDEditError
				require.ErrorAs(t, err, &refused)
				assert.Equal(t, &draft.MarkerIDEditError{At: f2At, ID: "f1", Restore: "f2"}, refused)
				assert.Equal(t, want, hintFor(err))
			}
		})
	}
}
