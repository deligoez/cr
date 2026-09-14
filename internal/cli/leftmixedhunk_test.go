package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// mixedHunkHome is a checkout whose pull request forms one hunk that both adds
// and removes lines, briefed through `cr brief`, so the unit is the one the
// command forms from the diff.
//
// mixed.go gains two lines after base line 9 and loses base lines 11 and 12.
// With three lines of context that is one hunk, `@@ -7,9 +7,9 @@`: head lines 10
// and 11 are the added ones, and base lines 11 and 12 the removed ones, which
// head lines 11 and 12 do not hold, so a payload numbered in the head would
// differ from one numbered in the merge base.
func mixedHunkHome(t *testing.T) state.Layout {
	t.Helper()
	dir := t.TempDir()
	write := func(lines []string) {
		t.Helper()
		body := strings.Join(lines, "\n") + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "mixed.go"), []byte(body), 0o600))
	}
	base := numbered("mixed")
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write(base)
	mustGit(t, dir, "add", "mixed.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	head := append([]string{}, base[:9]...)
	head = append(head, "// mixed added a", "// mixed added b", base[9])
	write(append(head, base[12:]...))
	mustGit(t, dir, "commit", "--quiet", "-am", "the change under review")
	headSHA := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	baseSHA := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), headSHA, baseSHA)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	for id, body := range profile.Builtins() {
		require.NoError(t, layout.EnsureProfile(id, body))
	}
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(fixtureIssue+": reshape the notes.\n"), 0o600))
	briefed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	require.NoError(t, err)

	var brief struct {
		Units []unit.Unit `json:"units"`
	}
	require.NoError(t, json.Unmarshal([]byte(briefed), &brief))
	require.Len(t, brief.Units, 1, "the fixture forms one unit")
	assert.Equal(t, [3]any{"u1", "mixed.go", git.Right},
		[3]any{brief.Units[0].ID, brief.Units[0].Path, brief.Units[0].Side},
		"a hunk that adds lines forms a RIGHT unit")
	return layout
}

// §9.2.1 and §7.2's location row through `cr draft` and `cr post`: a removed
// line of a hunk that also adds lines is a deletion the diff carries, so a
// marker moved from the unit's added head line onto base lines 11 and 12, with
// its side edited from RIGHT to LEFT, stays inside the record's unit and is
// accepted. The record is re-anchored on the merge base with §9.2's hash taken
// from there, the regenerated marker says LEFT, and the dry-run payload places
// the comment on the LEFT at the merge-base lines, not at the head lines of the
// same numbers. The anchors-on-changed-lines check `cr record` runs agrees: the
// same LEFT anchor given to `cr record` is stored.
func TestALeftEditOntoRemovedLinesOfAMixedHunkIsAccepted(t *testing.T) {
	onMixed := func(side string, start, line int) map[string]any {
		return onUnit("u1", "mixed.go", side, start, line)
	}

	t.Run("cr record agrees", func(t *testing.T) {
		layout := mixedHunkHome(t)
		_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", onMixed("LEFT", 11, 12)),
			"--repo", fixtureSlug)
		require.NoError(t, err)
		stored := storedFindings(t, layout)
		require.Len(t, stored, 1)
		assert.Equal(t, [4]any{"u1", git.Left, 11, 12},
			[4]any{stored[0].Unit, stored[0].Anchor.Side, stored[0].Anchor.StartLine, stored[0].Anchor.Line})
	})

	t.Run("cr draft and cr post", func(t *testing.T) {
		layout := mixedHunkHome(t)
		_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", onMixed("RIGHT", 10, 10)),
			"--repo", fixtureSlug)
		require.NoError(t, err)
		_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)
		require.NoError(t, err)
		drafted := layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileDraft)
		editF1Marker(t, drafted, `side="RIGHT" start_line="10" line="10"`, `side="LEFT" start_line="11" line="12"`)

		_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)

		require.NoError(t, err)
		stored := storedFindings(t, layout)
		require.Len(t, stored, 1)
		hash, err := finding.AnchorContentHash([]string{"// mixed 10", "// mixed 11"})
		require.NoError(t, err)
		assert.Equal(t, [5]any{"u1", git.Left, 11, 12, hash},
			[5]any{stored[0].Unit, stored[0].Anchor.Side, stored[0].Anchor.StartLine, stored[0].Anchor.Line,
				stored[0].Anchor.ContentHash},
			"re-anchored on merge-base lines 11 and 12 inside u1")
		at, line := f1Marker(t, drafted)
		marker, err := draft.ParseMarker(at, line)
		require.NoError(t, err)
		assert.Equal(t, [4]any{"mixed.go", "LEFT", 11, 12},
			[4]any{marker.Path, marker.Side, marker.StartLine, marker.Line}, "and the regenerated marker says so")

		printed, err := runPost(t, fixturePR, "--repo", fixtureSlug)

		require.NoError(t, err)
		var report struct {
			Payload *post.Review `json:"payload"`
		}
		require.NoError(t, json.Unmarshal([]byte(printed), &report))
		require.NotNil(t, report.Payload)
		require.Len(t, report.Payload.Comments, 1)
		comment := report.Payload.Comments[0]
		assert.Equal(t, [5]any{"mixed.go", 11, git.Left, 12, git.Left},
			[5]any{comment.Path, comment.StartLine, comment.StartSide, comment.Line, comment.Side})
	})
}
