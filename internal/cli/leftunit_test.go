package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// removalHome is a checkout whose pull request only removes lines, briefed
// through `cr brief`, so the units are the ones the command forms from the diff.
//
// lib.go loses merge-base lines 7 and 8 of fourteen. With three lines of
// context its one hunk is `@@ -4,8 +4,6 @@`: head lines 4 to 9 are context, and
// head line 6 is the line the removal follows. gone.go, three lines, is deleted
// whole, `@@ -1,3 +0,0 @@`. The brief's document is returned beside the state.
func removalHome(t *testing.T) (layout state.Layout, briefed string) {
	t.Helper()
	dir := t.TempDir()
	lib := []string{
		"package lib", "", "func Load() {", "\ta := 1", "\tb := 2", "\tc := 3",
		"\tpanic(\"gone\")", "\tpanic(\"also gone\")", "\t_ = a", "\t_ = b", "\t_ = c", "}", "", "func Keep() {}",
	}
	write := func(name string, lines []string) {
		t.Helper()
		body := strings.Join(lines, "\n") + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("lib.go", lib)
	write("gone.go", []string{"package lib", "", "func Gone() {}"})
	mustGit(t, dir, "add", "lib.go", "gone.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("lib.go", append(append([]string{}, lib[:6]...), lib[8:]...))
	mustGit(t, dir, "rm", "--quiet", "gone.go")
	mustGit(t, dir, "commit", "--quiet", "-am", "the change under review only removes")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout = state.New(crHome(t))
	require.NoError(t, layout.Init())
	for id, body := range profile.Builtins() {
		require.NoError(t, layout.EnsureProfile(id, body))
	}
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(fixtureIssue+": drop the panics.\n"), 0o600))
	briefed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	require.NoError(t, err)
	return layout, briefed
}

// QA D-W1-1 through `cr brief` and `cr review`: a unit made only of removed
// lines reports its hunk ranges as the merge-base lines it removes — never the
// head-side header range, and for a deleted file never 0-0 — so the LEFT anchor
// the prompt asks for counts where the prompt's schema says. Its head ranges
// are the insertion points §6.2.1 measures in.
func TestADeletionOnlyUnitReportsTheMergeBaseLinesItRemoves(t *testing.T) {
	_, printed := removalHome(t)

	var briefed struct {
		Units []unit.Unit `json:"units"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &briefed))
	type shape struct {
		ID, Path    string
		Side        git.Side
		Hunk, Heads []unit.Range
	}
	got := make([]shape, 0, len(briefed.Units))
	for _, formed := range briefed.Units {
		got = append(got, shape{formed.ID, formed.Path, formed.Side, formed.HunkRanges, formed.HeadRanges})
	}
	assert.Equal(t, []shape{
		{"u1", "gone.go", git.Left, []unit.Range{{Start: 1, End: 3}}, []unit.Range{{Start: 0, End: 0}}},
		{"u2", "lib.go", git.Left, []unit.Range{{Start: 7, End: 8}}, []unit.Range{{Start: 6, End: 6}}},
	}, got)

	said := map[string]string{}
	for _, prompt := range fanOut(t, "--axis", axis.Intent).Prompts {
		said[prompt.Unit] = prompt.Text
	}
	require.Len(t, said, 2)
	assert.Contains(t, said["u1"], "gone.go, LEFT side, formed by adjacency, 3 changed line(s), hunk ranges 1-3.\n")
	assert.Contains(t, said["u2"], "lib.go, LEFT side, formed by adjacency, 2 changed line(s), hunk ranges 7-8.\n")
}

// QA D-W1-2 through `cr record`: a deletion-only unit's own lines for §6.2.1
// are its removed lines and its insertion point, so a citation to the unchanged
// head line above or below the removal lies outside the unit and grades
// `cited`, and one at the insertion point lies inside it and grades `argued`.
func TestACitationBesideADeletionIsOutsideItsUnit(t *testing.T) {
	layout, _ := removalHome(t)
	citing := func(id string, line int) map[string]any {
		record := aRecord(id, "u2")
		record["class"] = "citation-at-" + id
		record["anchor"] = map[string]any{
			"path": "lib.go", "side": "LEFT", "start_line": 7, "line": 8, "content_hash": "0123456789abcdef",
		}
		record["citations"] = []map[string]any{{"path": "lib.go", "line": line}}
		return record
	}
	file := writeRecordFile(t, "merged.ndjson",
		citing("f1", 4), citing("f2", 6), citing("f3", 7), citing("f4", 9))

	_, err := runRecord(t, fixturePR, file, "--repo", fixtureSlug)
	require.NoError(t, err)

	graded := map[string][2]string{}
	for _, stored := range storedFindings(t, layout) {
		graded[stored.ID] = [2]string{string(stored.Grade), string(stored.Kind)}
	}
	assert.Equal(t, map[string][2]string{
		"f1": {string(finding.GradeCited), string(finding.KindFinding)},
		"f2": {string(finding.GradeArgued), string(finding.KindQuestion)},
		"f3": {string(finding.GradeCited), string(finding.KindFinding)},
		"f4": {string(finding.GradeCited), string(finding.KindFinding)},
	}, graded)
}
