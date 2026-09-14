package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/state"
)

// onDeletedFile is a question on removalHome's gone.go unit u1, which deletes
// the file whole, anchored on the LEFT across its three removed lines.
func onDeletedFile() map[string]any {
	record := onRemovalUnit("LEFT", 1, 3)
	record["unit"] = "u1"
	record["anchor"].(map[string]any)["path"] = "gone.go"
	return record
}

// sideDrafted is removalHome with record recorded and drafted, and the path of
// the round's draft.md.
func sideDrafted(t *testing.T, record map[string]any) (layout state.Layout, drafted string) {
	t.Helper()
	layout, _ = removalHome(t)
	_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", record), "--repo", fixtureSlug)
	require.NoError(t, err)
	_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	return layout, layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileDraft)
}

// f1Marker is the one-based number and the text of record f1's marker line in
// the draft at drafted.
func f1Marker(t *testing.T, drafted string) (at int, line string) {
	t.Helper()
	body, err := os.ReadFile(drafted)
	require.NoError(t, err)
	for i, text := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(text, `<!-- cr:record id="f1"`) {
			return i + 1, text
		}
	}
	require.Fail(t, "no marker", "the draft holds no block for f1")
	return 0, ""
}

// editF1Marker applies markerEdit to record f1's marker in the draft at
// drafted, on disk.
func editF1Marker(t *testing.T, drafted, from, to string) {
	t.Helper()
	body, err := os.ReadFile(drafted)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(drafted, []byte(markerEdit(t, string(body), "f1", from, to)), 0o600))
}

// §7.1.1 through `cr draft` and `cr post`: a LEFT record on a deletion is
// rendered with `side="LEFT"` between `path` and `start_line`, the draft reads
// back to the record's own fields, and the draft left unchanged posts the
// comment where the record is anchored, on the LEFT.
func TestADraftMarkerCarriesTheSideAndAnUnchangedDraftPostsAsRendered(t *testing.T) {
	layout, drafted := sideDrafted(t, onRemovalUnit("LEFT", 7, 8))
	stored := storedFindings(t, layout)
	require.Len(t, stored, 1)

	at, line := f1Marker(t, drafted)
	marker, err := draft.ParseMarker(at, line)
	require.NoError(t, err)
	assert.Equal(t, draft.Marker{
		ID: "f1", Kind: string(finding.KindQuestion), Path: "lib.go", Side: "LEFT", StartLine: 7, Line: 8,
		Severity: string(stored[0].Severity), Grade: string(stored[0].Grade), Disposition: "",
	}, marker)

	printed, err := runPost(t, fixturePR, "--repo", fixtureSlug)

	require.NoError(t, err)
	var report struct {
		Payload post.Review `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	require.Len(t, report.Payload.Comments, 1)
	comment := report.Payload.Comments[0]
	assert.Equal(t, [5]any{"lib.go", 7, git.Left, 8, git.Left},
		[5]any{comment.Path, comment.StartLine, comment.StartSide, comment.Line, comment.Side})
	assert.Equal(t, stored[0].Anchor, storedFindings(t, layout)[0].Anchor, "an unchanged marker moves nothing")
}

// §7.2's `side` row through `cr draft`: an edited side is re-validated per
// §6.1.2 like the lines are. Moving a LEFT anchor on a deleted file to the
// RIGHT names a file the head does not hold, so the draft aborts with exit 1
// naming the record and stores nothing; moving one on lines the head still
// holds re-anchors the record there, with §9.2's hash taken from the head.
func TestAnEditedSideIsReValidatedAgainstTheTreeItNames(t *testing.T) {
	t.Run("a side that no longer resolves", func(t *testing.T) {
		layout, drafted := sideDrafted(t, onDeletedFile())
		before := storedFindings(t, layout)
		at, _ := f1Marker(t, drafted)
		editF1Marker(t, drafted, `side="LEFT"`, `side="RIGHT"`)

		_, err := runDraft(t, fixturePR, "--repo", fixtureSlug)

		var refused *draft.MarkerEditError
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, draft.MarkerEditError{
			ID: "f1", At: at, Field: "anchor",
			Problem: `names "gone.go", which the head under review does not hold as a file`,
		}, *refused)
		assert.Equal(t, ExitValidation, exitCodeFor(err))
		assert.Equal(t, before, storedFindings(t, layout), "a refused edit stores nothing")
	})

	t.Run("a side that resolves", func(t *testing.T) {
		layout, drafted := sideDrafted(t, onRemovalUnit("LEFT", 7, 8))
		editF1Marker(t, drafted, `side="LEFT"`, `side="RIGHT"`)

		_, err := runDraft(t, fixturePR, "--repo", fixtureSlug)

		require.NoError(t, err)
		stored := storedFindings(t, layout)
		require.Len(t, stored, 1)
		hash, err := finding.AnchorContentHash([]string{"\t_ = a", "\t_ = b"})
		require.NoError(t, err)
		assert.Equal(t, [4]any{git.Right, 7, 8, hash},
			[4]any{stored[0].Anchor.Side, stored[0].Anchor.StartLine, stored[0].Anchor.Line, stored[0].Anchor.ContentHash},
			"re-anchored on the head's lines 7 and 8")
		at, line := f1Marker(t, drafted)
		marker, err := draft.ParseMarker(at, line)
		require.NoError(t, err)
		assert.Equal(t, "RIGHT", marker.Side, "and the regenerated marker says so")
	})
}

// §7.1.1 against a draft written before `side` joined the marker: both commands
// that read the draft refuse it with exit 1, naming the record and quoting the
// grammar, and no side is assumed for it.
func TestAMarkerWithoutASideIsRefusedNamingTheRecordAndTheGrammar(t *testing.T) {
	for name, run := range runsReadingTheDraft {
		t.Run(name, func(t *testing.T) {
			layout, drafted := sideDrafted(t, onRemovalUnit("LEFT", 7, 8))
			editF1Marker(t, drafted, ` side="LEFT"`, "")
			at, line := f1Marker(t, drafted)

			_, err := run(t, fixturePR, "--repo", fixtureSlug)

			var malformed *draft.MalformedMarkerError
			require.ErrorAs(t, err, &malformed)
			problem := `" side=" does not follow, and §7.1.1 fixes the nine fields and their order`
			assert.Equal(t, draft.MalformedMarkerError{At: at, ID: "f1", Line: line, Problem: problem}, *malformed)
			assert.Equal(t, fmt.Sprintf(
				"draft line %d, record f1, is a malformed record marker: %s\n  read: %s\n  grammar: %s",
				at, problem, line, draft.MarkerGrammar), malformed.Error())
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Equal(t, finding.StateQueued, storedFindings(t, layout)[0].State, "nothing was triaged")
		})
	}
}
