package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// twoHunkHome is deletingHome with its one unit recorded as two hunk ranges,
// head lines 1 to 2 and 5 to 6, so head lines 3 and 4 lie between the unit's
// hunks and inside neither.
//
// The diff is deletingHome's own: base line 4 is still the one line it
// removes, which is what the LEFT case below is measured against.
func twoHunkHome(t *testing.T) state.Layout {
	t.Helper()
	layout := deletingHome(t)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":1,"end":2},{"start":5,"end":6}],`+
			`"head":"`+meta.Head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// anchoredOn is aRecord on twoHunkHome's unit, anchored on side from start to
// line.
func anchoredOn(side string, start, line int) map[string]any {
	record := aRecord("f1", "u1")
	record["anchor"] = map[string]any{
		"path": "lib.go", "side": side, "start_line": start, "line": line,
		"content_hash": "0123456789abcdef",
	}
	return record
}

// foreignAnchorProblem is the whole refusal an anchor outside its unit draws,
// for the anchor text at.
func foreignAnchorProblem(at string) string {
	return "of record f1 is " + at + `, which does not lie inside unit "u1", the unit this record names; ` +
		"§6.1.3 has a record's anchor lie inside its unit under §6.2.1's containment, " +
		"so name the unit whose hunk holds it"
}

// QA S10: `cr merge` accepted a RIGHT anchor spanning two hunks of its unit and
// `cr record` refused the file it wrote, one command late. Every anchor
// `cr record` refuses is now refused by `cr merge` on the same round, with the
// same field and problem on the role file's own line, and nothing is written;
// an anchor the merge accepts is accepted again when its output is recorded.
func TestMergeRefusesEveryAnchorRecordRefuses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		record  map[string]any
		problem string
	}{
		{
			name:    "a range spanning two hunks of its unit",
			record:  anchoredOn("RIGHT", 2, 5),
			problem: foreignAnchorProblem("RIGHT lib.go:2-5"),
		},
		{
			name:    "a range outside its unit",
			record:  anchoredOn("RIGHT", 8, 8),
			problem: foreignAnchorProblem("RIGHT lib.go:8-8"),
		},
		{
			name:    "a LEFT anchor on a line the diff did not remove",
			record:  anchoredOn("LEFT", 5, 5),
			problem: unremovedLeftProblem("LEFT lib.go:5-5"),
		},
		{name: "a range inside one hunk of its unit", record: anchoredOn("RIGHT", 5, 6)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout := twoHunkHome(t)
			fanOut := writeFanOut(t, t.TempDir(), "correctness", tc.record)
			out := mergedOut(t)

			_, mergeErr := runCLIPrinting(t, "merge", fanOut, "-o", out, "--repo", fixtureSlug, "--pr", fixturePR)

			if tc.problem == "" {
				require.NoError(t, mergeErr)
				_, err := runRecord(t, fixturePR, out, "--repo", fixtureSlug)
				require.NoError(t, err, "a merged file is recorded when its anchors were merged")
				assert.Len(t, storedFindings(t, layout), 1)
				return
			}
			direct := writeRecordFile(t, "merged.ndjson", tc.record)
			_, recordErr := runRecord(t, fixturePR, direct, "--repo", fixtureSlug)

			var merged, recorded *finding.RejectedRecordError
			require.ErrorAs(t, mergeErr, &merged)
			require.ErrorAs(t, recordErr, &recorded)
			assert.Equal(t, finding.RejectedRecordError{
				File: fanOut, Line: 1, Field: "anchor", Problem: tc.problem,
			}, *merged)
			assert.Equal(t, finding.RejectedRecordError{
				File: direct, Line: 1, Field: "anchor", Problem: tc.problem,
			}, *recorded)
			assert.Equal(t, ExitValidation, exitCodeFor(mergeErr))
			assert.Equal(t, ExitValidation, exitCodeFor(recordErr))
			_, statErr := os.Stat(out)
			assert.ErrorIs(t, statErr, os.ErrNotExist, "a refused merge writes no output")
			assert.Empty(t, storedFindings(t, layout), "a refused file stores none of its records")
		})
	}
}
