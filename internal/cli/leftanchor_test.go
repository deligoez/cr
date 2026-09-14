package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// deletingHome is a checkout whose pull request removes one line and adds none,
// and a state root holding that pull request briefed at round 1 with the one
// unit the change forms, behind a `gh` shim answering with the checkout's own
// two revisions.
//
// The change removes base line 4 of lib.go's twelve. §3.4.1's diff carries three
// lines of context, so its one hunk is `@@ -1,7 +1,6 @@`: base lines 1 to 3 and
// 5 to 7 are context inside the hunk, base line 4 is the only removed line, and
// base lines 8 to 12 lie outside every hunk. The unit is the hunk's head-side
// range, lines 1 to 6.
func deletingHome(t *testing.T) state.Layout {
	t.Helper()
	dir := t.TempDir()
	lines := []string{
		"package lib", "", "func Load() {", "\tpanic(\"gone\")", "}", "",
		"// a", "// b", "// c", "// d", "// e", "func Keep() {}",
	}
	write := func(kept []string) {
		t.Helper()
		body := strings.Join(kept, "\n") + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lib.go"), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write(lines)
	mustGit(t, dir, "add", "lib.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write(append(append([]string{}, lines[:3]...), lines[4:]...))
	mustGit(t, dir, "commit", "--quiet", "-am", "the change under review removes a line")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber, Round: 1, Head: head,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"lib.go","hunk_ranges":[{"start":1,"end":6}],`+
			`"head":"`+head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// leftAnchored is aRecord on deletingHome's unit, anchored on the LEFT side from
// start to line.
func leftAnchored(start, line int) map[string]any {
	record := aRecord("f1", "u1")
	record["anchor"] = map[string]any{
		"path": "lib.go", "side": "LEFT", "start_line": start, "line": line,
		"content_hash": "0123456789abcdef",
	}
	return record
}

// leftAnchorCases are the LEFT anchors §9.2.1 has to tell apart on
// deletingHome's diff, with the anchor text a refusal names, empty for the one
// anchor that names removed lines only.
var leftAnchorCases = []struct {
	name        string
	start, line int
	refused     string
}{
	{name: "the removed line", start: 4, line: 4},
	{name: "a context line inside the hunk", start: 5, line: 5, refused: "LEFT lib.go:5-5"},
	{name: "a line outside every hunk", start: 11, line: 11, refused: "LEFT lib.go:11-11"},
	{name: "the removed line and the context line after it", start: 4, line: 5, refused: "LEFT lib.go:4-5"},
}

// unremovedLeftProblem is the whole refusal a LEFT anchor on lines the diff did
// not all remove draws, for the anchor text at.
func unremovedLeftProblem(at string) string {
	return "of record f1 is " + at + ", and the round's diff does not remove every one of those " +
		"merge-base lines; §9.2.1 has a LEFT anchor name removed lines only, so anchor the lines a " +
		"hunk removes, or on the RIGHT a head line of a unit that adds lines"
}

// QA D-S05-2 through `cr record`: a LEFT anchor on a line the diff removed is
// recorded, and one on a context line of the hunk or on a line outside every
// hunk is refused with exit 1, naming the file's line and the record, and
// nothing is stored.
//
// The context line is the case that reached the payload: a hunk's merge-base
// range covers its context, and a LEFT anchor inside that range was taken for a
// removed line, recorded, drafted, and built into a comment GitHub's
// review-creation call refuses.
func TestRecordRefusesALeftAnchorOnALineTheDiffDidNotRemove(t *testing.T) {
	for _, tc := range leftAnchorCases {
		t.Run(tc.name, func(t *testing.T) {
			layout := deletingHome(t)
			file := writeRecordFile(t, "merged.ndjson", leftAnchored(tc.start, tc.line))

			_, err := runRecord(t, fixturePR, file, "--repo", fixtureSlug)

			if tc.refused == "" {
				require.NoError(t, err)
				stored := storedFindings(t, layout)
				require.Len(t, stored, 1)
				assert.Equal(t, git.Left, stored[0].Anchor.Side)
				assert.Equal(t, [2]int{4, 4}, [2]int{stored[0].Anchor.StartLine, stored[0].Anchor.Line})
				return
			}
			var rejected *finding.RejectedRecordError
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, finding.RejectedRecordError{
				File: file, Line: 1, Field: "anchor", Problem: unremovedLeftProblem(tc.refused),
			}, *rejected)
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Empty(t, storedFindings(t, layout), "a refused file stores none of its records")
		})
	}
}

// The same refusal through `cr merge`, which reads the role's own file before
// `cr record` ever does: a LEFT anchor on a line the diff did not remove is
// refused there, naming the line, and no output file is written.
func TestMergeRefusesALeftAnchorOnALineTheDiffDidNotRemove(t *testing.T) {
	for _, tc := range leftAnchorCases {
		t.Run(tc.name, func(t *testing.T) {
			deletingHome(t)
			file := writeFanOut(t, t.TempDir(), "correctness", leftAnchored(tc.start, tc.line))
			out := mergedOut(t)

			_, err := runCLIPrinting(t, "merge", file, "-o", out, "--repo", fixtureSlug, "--pr", fixturePR)

			if tc.refused == "" {
				require.NoError(t, err)
				assert.FileExists(t, out)
				return
			}
			var rejected *finding.RejectedRecordError
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, finding.RejectedRecordError{
				File: file, Line: 1, Field: "anchor", Problem: unremovedLeftProblem(tc.refused),
			}, *rejected)
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.NoFileExists(t, out, "a refused merge writes no output")
		})
	}
}
