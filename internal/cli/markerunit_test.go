package cli

import (
	"encoding/json"
	"fmt"
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

// numbered is a file of lines: `package lib`, then `// <name> 1` to
// `// <name> 20`, so base line n+1 holds `// <name> n`.
func numbered(name string) []string {
	lines := []string{"package lib"}
	for n := 1; n <= 20; n++ {
		lines = append(lines, fmt.Sprintf("// %s %d", name, n))
	}
	return lines
}

// threeUnitHome is a checkout whose pull request forms three units, briefed
// through `cr brief`, so the units are the ones the command forms from the diff.
//
// money.go and tax.go each gain two lines after base line 11, so head lines 12
// and 13 of each are lines the diff added: u1 and u3, on the RIGHT. order.go
// loses base lines 11 and 12 and gains none: u2, on the LEFT, whose insertion
// point is head line 10.
func threeUnitHome(t *testing.T) state.Layout {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, lines []string) {
		t.Helper()
		body := strings.Join(lines, "\n") + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	added := func(name string) []string {
		base := numbered(name)
		head := append([]string{}, base[:11]...)
		head = append(head, "// "+name+" added a", "// "+name+" added b")
		return append(head, base[11:]...)
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	for _, name := range []string{"money", "order", "tax"} {
		write(name+".go", numbered(name))
	}
	mustGit(t, dir, "add", "money.go", "order.go", "tax.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("money.go", added("money"))
	write("tax.go", added("tax"))
	order := numbered("order")
	write("order.go", append(append([]string{}, order[:10]...), order[12:]...))
	mustGit(t, dir, "commit", "--quiet", "-am", "the change under review")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	for id, body := range profile.Builtins() {
		require.NoError(t, layout.EnsureProfile(id, body))
	}
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(fixtureIssue+": halve the money.\n"), 0o600))
	briefed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	require.NoError(t, err)

	var brief struct {
		Units []unit.Unit `json:"units"`
	}
	require.NoError(t, json.Unmarshal([]byte(briefed), &brief))
	formed := make(map[string][2]string, len(brief.Units))
	for i := range brief.Units {
		formed[brief.Units[i].ID] = [2]string{brief.Units[i].Path, string(brief.Units[i].Side)}
	}
	require.Equal(t, map[string][2]string{
		"u1": {"money.go", "RIGHT"}, "u2": {"order.go", "LEFT"}, "u3": {"tax.go", "RIGHT"},
	}, formed, "the fixture forms the three units its tests name")
	return layout
}

// onUnit is a record of threeUnitHome's round on unit id, anchored on side of
// path from start to line, citing cited when it is not empty. Its summary asks,
// so §8.1.5 posts it whichever register the grade leaves it in.
func onUnit(id, path, side string, start, line int, cited ...map[string]any) map[string]any {
	record := aRecord("f1", id)
	record["summary"] = "Does the total round the tax twice?"
	record["anchor"] = map[string]any{
		"path": path, "side": side, "start_line": start, "line": line, "content_hash": "0123456789abcdef",
	}
	if len(cited) > 0 {
		record["citations"] = cited
	}
	return record
}

// recordedIn is what `cr record` makes of record in a threeUnitHome of its own:
// the stored record, or the RejectedRecordError's Problem.
func recordedIn(t *testing.T, record map[string]any) (stored *finding.Finding, problem string) {
	t.Helper()
	layout := threeUnitHome(t)
	_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", record), "--repo", fixtureSlug)
	if err != nil {
		var rejected *finding.RejectedRecordError
		require.ErrorAs(t, err, &rejected)
		return nil, rejected.Problem
	}
	records := storedFindings(t, layout)
	require.Len(t, records, 1)
	return &records[0], ""
}

// unitDrafted is threeUnitHome with record recorded and drafted, and the path of
// the round's draft.md.
func unitDrafted(t *testing.T, record map[string]any) (layout state.Layout, drafted string) {
	t.Helper()
	layout = threeUnitHome(t)
	_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", record), "--repo", fixtureSlug)
	require.NoError(t, err)
	_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	return layout, layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileDraft)
}

// outsideUnitHint is the hint §7.2's location row gives a marker moved out of
// its record's unit.
const outsideUnitHint = "§6.1.3 keeps a record's anchor inside its own unit, which no marker edit changes; " +
	"move the marker back onto lines of that unit, or delete the block. A comment on another unit needs a " +
	"record a role produced for that unit, since §7.2.3 gives the draft no manual-comment channel"

// QA D-V4-1 and D-V4-2 through `cr draft` and `cr post`: §7.2's location row
// re-validates a moved marker per §6.1.3, so an anchor moved out of the record's
// unit is refused with exit 1 naming the record, in the words `cr record` refuses
// the same anchor with up to their last clause, which is the marker's own way
// forward rather than advice to name a unit (QA D-V5-2), and nothing is stored,
// rewritten or put in front of the review call.
//
// D-V4-1 is the cited finding moved onto the very line its one citation lies
// on, in another unit, which would have posted as an assertion: `cr record`
// grades that anchor `argued` when the record names the unit that holds it.
// D-V4-2 is the move from a deletion-only unit onto another unit's added lines,
// which the side check measured against the old unit and refused for a reason
// false of the new position; it is refused as leaving the unit.
func TestAMarkerMovedIntoAnotherUnitIsRefused(t *testing.T) {
	type place struct {
		path, side  string
		start, line int
	}
	marker := func(at place) string {
		return fmt.Sprintf(`path=%q side=%q start_line="%d" line="%d"`, at.path, at.side, at.start, at.line)
	}
	for _, tc := range []struct {
		name     string
		unit     string
		cited    []map[string]any
		from, to place
		// recorded is `cr record`'s Problem for the anchor the marker moves
		// to, and refusal the marker's: the same words up to the last clause,
		// which is each command's own way forward (QA D-V5-2).
		recorded, refusal string
	}{
		{
			name: "D-V4-1 a cited finding moved onto the line it cites",
			unit: "u1", cited: []map[string]any{{"path": "tax.go", "line": 12}},
			from: place{"money.go", "RIGHT", 12, 12}, to: place{"tax.go", "RIGHT", 12, 12},
			recorded: `of record f1 is RIGHT tax.go:12-12, which does not lie inside unit "u1", the unit this ` +
				"record names; §6.1.3 has a record's anchor lie inside its unit under §6.2.1's containment, " +
				"so name the unit whose hunk holds it",
			refusal: `of record f1 is RIGHT tax.go:12-12, which does not lie inside unit "u1", the unit this ` +
				"record names; §6.1.3 has a record's anchor lie inside its unit under §6.2.1's containment, " +
				"so keep the comment within its unit or delete the block",
		},
		{
			name: "D-V4-2 a record on a deletion moved onto lines another unit adds",
			unit: "u2",
			from: place{"order.go", "LEFT", 11, 12}, to: place{"money.go", "RIGHT", 12, 12},
			recorded: `of record f1 is RIGHT money.go:12-12, which does not lie inside unit "u2", the unit this ` +
				"record names; §6.1.3 has a record's anchor lie inside its unit under §6.2.1's containment, " +
				"so name the unit whose hunk holds it",
			refusal: `of record f1 is RIGHT money.go:12-12, which does not lie inside unit "u2", the unit this ` +
				"record names; §6.1.3 has a record's anchor lie inside its unit under §6.2.1's containment, " +
				"so keep the comment within its unit or delete the block",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := onUnit(tc.unit, tc.from.path, tc.from.side, tc.from.start, tc.from.line, tc.cited...)
			_, recorded := recordedIn(t,
				onUnit(tc.unit, tc.to.path, tc.to.side, tc.to.start, tc.to.line, tc.cited...))
			require.Equal(t, tc.recorded, recorded, "cr record refuses the anchor the marker moves to")

			for name, command := range map[string][]string{
				"draft": {"draft", fixturePR, "--repo", fixtureSlug},
				"post":  {"post", fixturePR, "--repo", fixtureSlug},
				"post --confirm": {
					"post", fixturePR, "--repo", fixtureSlug, "--confirm",
				},
			} {
				t.Run(strings.ReplaceAll(name, " --", "-"), func(t *testing.T) {
					layout, drafted := unitDrafted(t, record)
					before := storedFindings(t, layout)
					posted := layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FilePosted)
					sent, err := os.ReadFile(posted)
					require.NoError(t, err)
					at, _ := f1Marker(t, drafted)
					editF1Marker(t, drafted, marker(tc.from), marker(tc.to))
					edited, err := os.ReadFile(drafted)
					require.NoError(t, err)

					printed, err := runCLIPrinting(t, command...)

					var refused *draft.MarkerUnitEditError
					require.ErrorAs(t, err, &refused)
					assert.Equal(t, draft.MarkerEditError{ID: "f1", At: at, Field: "anchor", Problem: tc.refusal},
						*refused.MarkerEditError)
					assert.Equal(t, fmt.Sprintf("draft line %d, record f1: anchor %s", at, tc.refusal), err.Error())
					assert.Equal(t, ExitValidation, exitCodeFor(err))
					assert.Equal(t, outsideUnitHint, hintFor(err))
					assert.Empty(t, printed, "the run refused before it reported a payload")
					assert.Equal(t, before, storedFindings(t, layout), "a refused edit stores nothing")
					after, err := os.ReadFile(drafted)
					require.NoError(t, err)
					assert.Equal(t, string(edited), string(after), "and leaves the reviewer's draft as they saved it")
					now, err := os.ReadFile(posted)
					require.NoError(t, err)
					assert.Equal(t, string(sent), string(now), "nothing was put in front of the call")
				})
			}
		})
	}
}

// Why D-V4-1 mattered: the anchor the refused move names is one `cr record`
// grades `argued`, and so forces to a question, when the record names the unit
// that holds it — the citation then lies inside the record's own unit. The
// record the move started from was graded `cited`.
func TestTheRefusedMoveNamesAnAnchorItsOwnUnitGradesArgued(t *testing.T) {
	citation := map[string]any{"path": "tax.go", "line": 12}

	where, problem := recordedIn(t, onUnit("u1", "money.go", "RIGHT", 12, 12, citation))
	require.Empty(t, problem)
	assert.Equal(t, [2]any{finding.GradeCited, finding.KindFinding}, [2]any{where.Grade, where.Kind})

	moved, problem := recordedIn(t, onUnit("u3", "tax.go", "RIGHT", 12, 12, citation))
	require.Empty(t, problem)
	assert.Equal(t, [2]any{finding.GradeArgued, finding.KindQuestion}, [2]any{moved.Grade, moved.Kind})
}

// §7.2's location row and §7.2.2 through `cr draft` and `cr post`: a marker
// moved onto another line of the record's own unit re-anchors the record, and
// its grade is computed again, so the payload carries the moved position in the
// register the grade allows. A citation outside the unit keeps the finding
// `cited`; one inside the unit leaves it `argued`, forced to a question.
func TestAMarkerMovedWithinItsUnitReanchorsAndIsRegraded(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cited map[string]any
		grade finding.Grade
		kind  finding.Kind
	}{
		{name: "a citation in another unit", cited: map[string]any{"path": "tax.go", "line": 12},
			grade: finding.GradeCited, kind: finding.KindFinding},
		{name: "a citation inside the unit", cited: map[string]any{"path": "money.go", "line": 13},
			grade: finding.GradeArgued, kind: finding.KindQuestion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout, drafted := unitDrafted(t, onUnit("u1", "money.go", "RIGHT", 12, 12, tc.cited))
			editF1Marker(t, drafted, `start_line="12" line="12"`, `start_line="13" line="13"`)

			_, err := runDraft(t, fixturePR, "--repo", fixtureSlug)
			require.NoError(t, err)
			stored := storedFindings(t, layout)
			require.Len(t, stored, 1)
			hash, err := finding.AnchorContentHash([]string{"// money added b"})
			require.NoError(t, err)
			assert.Equal(t, [5]any{"u1", git.Right, 13, hash, tc.grade},
				[5]any{stored[0].Unit, stored[0].Anchor.Side, stored[0].Anchor.Line, stored[0].Anchor.ContentHash,
					stored[0].Grade},
				"re-anchored inside u1, the record's unit unchanged, and graded again")

			printed, err := runPost(t, fixturePR, "--repo", fixtureSlug)
			require.NoError(t, err)
			var report struct {
				Payload  *post.Review    `json:"payload"`
				Comments []postedComment `json:"comments"`
			}
			require.NoError(t, json.Unmarshal([]byte(printed), &report))
			assert.Equal(t, []postedComment{{ID: "f1", Kind: tc.kind}}, report.Comments)
			require.NotNil(t, report.Payload)
			require.Len(t, report.Payload.Comments, 1)
			comment := report.Payload.Comments[0]
			assert.Equal(t, [3]any{"money.go", 13, git.Right}, [3]any{comment.Path, comment.Line, comment.Side})
		})
	}
}
