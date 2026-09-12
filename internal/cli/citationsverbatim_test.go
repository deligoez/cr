package cli

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// §6.2.6 through the commands: every citation a `cited` record carries reaches
// draft.md as `path:line`, in the order the record stores them.
//
// The record carries three, and that is the whole point of the case. cr cannot
// judge whether a citation supports the summary and §6.2.6 has it render every
// one so the human can, which is a claim about all of them — a renderer that
// showed the first, the last, or the ones it found interesting would satisfy
// every other case in this package, since each of those queues a record with
// exactly one citation.
//
// What is asserted is the stored citations rather than the ones the fixture
// wrote. §6.2.3 resolves each entry against the head and §6.2.5 stamps its
// origin, so the record on disk is not the record that arrived, and §6.2.6 is
// about what the draft shows of what cr kept.
//
// The region is then compared whole, opening marker to closing. Containment of
// each line alone cannot see an order the record did not store, or a line cr
// put between two citations — and the block is what the author reads before
// deciding whether the summary stands.
func TestEveryStoredCitationOfACitedRecordReachesTheDraftBlock(t *testing.T) {
	layout := gradedHome(t)
	cited := aGradedRecord("f1")
	// Three locations the head holds, none inside the record's own unit —
	// §6.2's `cited` row asks for a location the human can check the
	// summary against, and app.go:3 is the code the record is about.
	cited["citations"] = []map[string]any{
		{"path": "app.go", "line": 1},
		{"path": ".gitignore", "line": 1},
		{"path": "app.go", "line": 2},
	}

	_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", cited), "--repo", fixtureSlug)
	require.NoError(t, err)
	stored, err := state.ReadRecords[finding.Finding](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, finding.GradeCited, stored[0].Grade,
		"the record has to actually be graded cited for §6.2.6 to bind it")
	require.Len(t, stored[0].Citations, 3, "cr stored every citation that arrived")

	_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	drafted, err := os.ReadFile(
		layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileDraft))
	require.NoError(t, err)

	block := blockOf(t, string(drafted), "f1")
	rendered := make([]string, 0, len(stored[0].Citations))
	for _, citation := range stored[0].Citations {
		line := fmt.Sprintf("citation: %s:%d", citation.Path, citation.Line)
		assert.Contains(t, block, line, "§6.2.6: this stored citation reaches the draft block")
		rendered = append(rendered, line)
	}
	assert.Contains(t, block,
		"<!-- cr:evidence -->\n"+strings.Join(rendered, "\n")+"\n<!-- cr:/evidence -->",
		"§6.2.6: all of them, in the order the record stores them, and nothing else")
}
