package cli

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
)

// recordedAnchorProblem is what `cr record` says of record, in a removalHome of
// its own: the RejectedRecordError's Problem, or "" when it records it.
func recordedAnchorProblem(t *testing.T, record map[string]any) string {
	t.Helper()
	removalHome(t)
	_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", record), "--repo", fixtureSlug)
	if err == nil {
		return ""
	}
	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	return rejected.Problem
}

// QA D-V2-1, D-V3-1 and D-V4-2 through `cr draft`: §7.2's location row
// re-validates a moved marker, and the anchor it moves to is held to the §6.1.3
// and §9.2.1 refusals `cr record` makes of the same anchor on removalHome's
// deletion-only unit u2. A LEFT anchor moved onto base line 9, a context line of
// the hunk, a RIGHT anchor moved onto head line 6, the insertion point inside
// the unit that only removes lines, and a RIGHT anchor moved onto head lines 7
// and 8, outside that unit, are each refused with exit 1 naming the record, in
// `cr record`'s words, and nothing is stored: findings.ndjson and the reviewer's
// draft stay as they were. Only the last is the refusal of a marker leaving its
// unit. A move `cr record` accepts, onto removed base line 8 alone, is
// re-anchored.
func TestAMarkerEditIsHeldToTheAnchorRulesOfRecord(t *testing.T) {
	from := `side="LEFT" start_line="7" line="8"`
	for _, tc := range []struct {
		name    string
		side    string
		start   int
		line    int
		problem string
		outside bool
	}{
		{
			name: "a LEFT anchor on a base line the diff did not remove", side: "LEFT", start: 9, line: 9,
			problem: "of record f1 is LEFT lib.go:9-9, and the round's diff does not remove every one of those " +
				"merge-base lines; §9.2.1 has a LEFT anchor name removed lines only, so anchor the lines a " +
				"hunk removes, or on the RIGHT a head line of a unit that adds lines",
		},
		{
			name: "a RIGHT anchor on the insertion point of a unit that only removes lines",
			side: "RIGHT", start: 6, line: 6,
			problem: `of record f1 is RIGHT lib.go:6-6, and unit "u2" only removes lines, so the diff changed no ` +
				"head line of it; §9.2.1 anchors a record about a deletion on the LEFT, so anchor the merge-base " +
				"lines the unit removes",
		},
		{
			name: "a RIGHT anchor outside the unit that only removes lines", side: "RIGHT", start: 7, line: 8,
			problem: `of record f1 is RIGHT lib.go:7-8, which does not lie inside unit "u2", the unit this record ` +
				"names; §6.1.3 has a record's anchor lie inside its unit under §6.2.1's containment, so name " +
				"the unit whose hunk holds it",
			outside: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.problem, recordedAnchorProblem(t, onRemovalUnit(tc.side, tc.start, tc.line)),
				"cr record refuses the anchor the marker moves to")
			layout, drafted := sideDrafted(t, onRemovalUnit("LEFT", 7, 8))
			before := storedFindings(t, layout)
			at, _ := f1Marker(t, drafted)
			editF1Marker(t, drafted, from,
				fmt.Sprintf(`side=%q start_line="%d" line="%d"`, tc.side, tc.start, tc.line))
			edited, err := os.ReadFile(drafted)
			require.NoError(t, err)

			_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)

			var refused *draft.MarkerEditError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, draft.MarkerEditError{ID: "f1", At: at, Field: "anchor", Problem: tc.problem}, *refused)
			var left *draft.MarkerUnitEditError
			assert.Equal(t, tc.outside, errors.As(err, &left), "refused as leaving the unit only when it does")
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Equal(t, before, storedFindings(t, layout), "a refused edit stores nothing")
			after, err := os.ReadFile(drafted)
			require.NoError(t, err)
			assert.Equal(t, string(edited), string(after), "and leaves the reviewer's draft as they saved it")
		})
	}

	t.Run("a move cr record accepts", func(t *testing.T) {
		require.Empty(t, recordedAnchorProblem(t, onRemovalUnit("LEFT", 8, 8)), "cr record accepts the anchor")
		layout, drafted := sideDrafted(t, onRemovalUnit("LEFT", 7, 8))
		editF1Marker(t, drafted, from, `side="LEFT" start_line="8" line="8"`)

		_, err := runDraft(t, fixturePR, "--repo", fixtureSlug)

		require.NoError(t, err)
		stored := storedFindings(t, layout)
		require.Len(t, stored, 1)
		hash, err := finding.AnchorContentHash([]string{"\tpanic(\"also gone\")"})
		require.NoError(t, err)
		assert.Equal(t, [4]any{git.Left, 8, 8, hash},
			[4]any{stored[0].Anchor.Side, stored[0].Anchor.StartLine, stored[0].Anchor.Line, stored[0].Anchor.ContentHash},
			"re-anchored on removed base line 8")
	})
}
