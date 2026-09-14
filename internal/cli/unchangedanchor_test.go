package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// onRemovalUnit is a question on removalHome's lib.go unit u2, anchored on side
// from start to line.
func onRemovalUnit(side string, start, line int) map[string]any {
	record := aRecord("f1", "u2")
	record["kind"] = "question"
	record["summary"] = "Does anything still call the removed panics?"
	record["anchor"] = map[string]any{
		"path": "lib.go", "side": side, "start_line": start, "line": line, "content_hash": "0123456789abcdef",
	}
	return record
}

// QA D-W1-3 through `cr record` and `cr merge`: u2 only removes merge-base lines
// 7 and 8, so every head line it reaches is one the diff did not change — head
// line 6, the insertion point it is measured at, as much as head line 4 above
// it. A RIGHT anchor naming it is refused, with exit 1 and the file's line,
// before it can be graded: on the insertion point, which lies inside u2, for
// the lines the diff did not change, and on head line 4, which does not, as
// lying outside the unit it names. So is a LEFT anchor on base line 9, a context
// line of the same hunk. The LEFT anchor on the removed lines is recorded.
func TestAnAnchorOnLinesADeletionDidNotChangeIsRefused(t *testing.T) {
	unchanged := func(at string) string {
		return "of record f1 is " + at + `, and unit "u2" only removes lines, so the diff changed no head line of it; ` +
			"§9.2.1 anchors a record about a deletion on the LEFT, so anchor the merge-base lines the unit removes"
	}
	for _, tc := range []struct {
		name    string
		record  map[string]any
		problem string
	}{
		{
			name:    "a RIGHT anchor on the insertion point",
			record:  onRemovalUnit("RIGHT", 6, 6),
			problem: unchanged("RIGHT lib.go:6-6"),
		},
		{
			name:   "a RIGHT anchor on a context line above the removal",
			record: onRemovalUnit("RIGHT", 4, 4),
			problem: `of record f1 is RIGHT lib.go:4-4, which does not lie inside unit "u2", the unit this record ` +
				"names; §6.1.3 has a record's anchor lie inside its unit under §6.2.1's containment, so name " +
				"the unit whose hunk holds it",
		},
		{
			name:   "a LEFT anchor on a context line below the removal",
			record: onRemovalUnit("LEFT", 9, 9),
			problem: "of record f1 is LEFT lib.go:9-9, and the round's diff does not remove every one of those " +
				"merge-base lines; §9.2.1 has a LEFT anchor name removed lines only, so anchor the lines a " +
				"hunk removes, or on the RIGHT a head line of a unit that adds lines",
		},
		{name: "a LEFT anchor on the removed lines", record: onRemovalUnit("LEFT", 7, 8)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout, _ := removalHome(t)
			fanOut := writeFanOut(t, t.TempDir(), "correctness", tc.record)
			out := mergedOut(t)

			_, mergeErr := runCLIPrinting(t, "merge", fanOut, "-o", out, "--repo", fixtureSlug, "--pr", fixturePR)
			direct := writeRecordFile(t, "merged.ndjson", tc.record)
			_, recordErr := runRecord(t, fixturePR, direct, "--repo", fixtureSlug)

			if tc.problem == "" {
				require.NoError(t, mergeErr)
				require.NoError(t, recordErr)
				stored := storedFindings(t, layout)
				require.Len(t, stored, 1)
				assert.Equal(t, [3]any{git.Left, 7, 8},
					[3]any{stored[0].Anchor.Side, stored[0].Anchor.StartLine, stored[0].Anchor.Line})
				return
			}
			var merged, recorded *finding.RejectedRecordError
			require.ErrorAs(t, mergeErr, &merged)
			require.ErrorAs(t, recordErr, &recorded)
			assert.Equal(t, finding.RejectedRecordError{
				File: fanOut, Line: 1, Field: "anchor", Problem: tc.problem,
			}, *merged)
			assert.Equal(t, finding.RejectedRecordError{
				File: direct, Line: 1, Field: "anchor", Problem: tc.problem,
			}, *recorded)
			assert.Equal(t, [2]int{ExitValidation, ExitValidation}, [2]int{exitCodeFor(mergeErr), exitCodeFor(recordErr)})
			assert.Empty(t, storedFindings(t, layout), "a refused record is never graded or stored")
		})
	}
}

// QA D-W1-3 through a marker edit and `cr post`: the reviewer moves a LEFT
// record's marker in draft.md from the removed lines onto base line 9, a
// context line of the same hunk, and `cr post` refuses it with exit 1 naming the
// record, dry run or confirmed, before any payload is reported or posted.json
// touched. It is refused where `cr post` reads the draft, by §7.2's location
// row, in the words `cr record` refuses the same anchor with.
func TestAMarkerMovedOntoALineTheDiffDidNotRemoveIsRefusedAtPost(t *testing.T) {
	for name, flags := range map[string][]string{"dry run": nil, "confirmed": {"--confirm"}} {
		t.Run(name, func(t *testing.T) {
			layout, _ := removalHome(t)
			_, err := runRecord(t, fixturePR,
				writeRecordFile(t, "merged.ndjson", onRemovalUnit("LEFT", 7, 8)), "--repo", fixtureSlug)
			require.NoError(t, err)
			_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)
			require.NoError(t, err)
			drafted := layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileDraft)
			posted := layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FilePosted)
			before, err := os.ReadFile(posted)
			require.NoError(t, err)
			body, err := os.ReadFile(drafted)
			require.NoError(t, err)
			moved := markerEdit(t, string(body), "f1", `start_line="7" line="8"`, `start_line="9" line="9"`)
			require.NoError(t, os.WriteFile(drafted, []byte(moved), 0o600))
			at, _ := f1Marker(t, drafted)

			printed, err := runPost(t, append([]string{fixturePR, "--repo", fixtureSlug}, flags...)...)

			var refused *draft.MarkerEditError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, draft.MarkerEditError{
				ID: "f1", At: at, Field: "anchor",
				Problem: "of record f1 is LEFT lib.go:9-9, and the round's diff does not remove every one of those " +
					"merge-base lines; §9.2.1 has a LEFT anchor name removed lines only, so anchor the lines a " +
					"hunk removes, or on the RIGHT a head line of a unit that adds lines",
			}, *refused)
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Empty(t, printed, "the run refused before it reported a payload")
			after, err := os.ReadFile(posted)
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after), "nothing was put in front of the call")
		})
	}
}
