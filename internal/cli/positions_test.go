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

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/suggestion"
)

// draftDiff is the diff draftedHome's round stands on: one hunk adding lines to
// the file aStoredRecord anchors in, wide enough to carry every anchor that
// fixture's tests write. Its head is no commit, so there is no diff to read.
var draftDiff = []git.Hunk{
	{Path: "internal/api/handler.go", Side: git.Right, BaseStart: 1, BaseLines: 100, HeadStart: 1, HeadLines: 100},
	{Path: "internal/api/second.go", Side: git.Right, BaseStart: 1, BaseLines: 100, HeadStart: 1, HeadLines: 100},
}

// standInDiff has §8.4.1's pre-validation read hunks instead of the round's
// diff, for a fixture whose head no checkout holds.
func standInDiff(t *testing.T, hunks []git.Hunk) {
	t.Helper()
	restore := positionHunks
	positionHunks = func(string, string, int, string) ([]git.Hunk, error) { return hunks, nil }
	t.Cleanup(func() { positionHunks = restore })
}

// suggesting is a record of the fixture's round carrying a replacement for the
// line it is anchored to.
//
// It is a question rather than a finding so that §6.3.3's refusal cannot stand
// in for §8.2.4's: a record with no citation recomputes to `argued`, and as a
// finding it would be refused before any position was looked at, which would
// leave this test passing for the wrong reason.
func suggesting(id string, line int, replacement string) *finding.Finding {
	return &finding.Finding{
		ID: id, Kind: finding.KindQuestion, Axis: "convention", Role: "convention",
		Class: "panic-in-library", Severity: finding.SeverityCritical,
		Grade: finding.GradeArgued, Unit: "u1",
		Anchor: finding.Anchor{
			Path: "lib.go", Side: git.Right,
			StartLine: line, Line: line, ContentHash: "0123456789abcdef",
		},
		Summary:    "Should Load return an error instead of panicking?",
		Evidence:   "The call aborts the caller's process.",
		Suggestion: replacement,
		State:      finding.StateDraft,
	}
}

// suggestingRound puts the fixture checkout of detectedHome behind CR_HOME with
// one record of the given shape already in findings.ndjson, drafted so the
// round holds it queued.
func suggestingRound(t *testing.T, record *finding.Finding) {
	t.Helper()
	layout := detectedHome(t)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, state.WriteStamped(
		held, state.FileFindings,
		state.Stamp{Head: record.Head, Round: 1}, []*finding.Finding{record},
	))
	require.NoError(t, held.Unlock())

	_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
}

// §8.4.1: the review-creation call is atomic, so every comment position is held
// to §8.2 before it, and a position §8.2 refuses stops the run.
//
// Nothing reaches the network here: the run carries no --confirm, so §8.5's
// gate mints no Confirmation for internal/gh's write door. What the test pins
// is that the refusal happens while the payload is being assembled, so the
// atomic call is never reached with a position that would lose the whole round.
func TestAnUnplaceablePositionStopsThePostBeforeAnythingIsSent(t *testing.T) {
	// Line 400 exists in no hunk of the fixture's diff, which spans lines
	// 3 to 7 of lib.go.
	suggestingRound(t, suggesting("f1", 400, "\treturn fmt.Errorf(\"one\")"))

	printed, err := runPost(t, fixturePR, "--repo", fixtureSlug)

	require.Error(t, err)
	var refused *suggestion.RangeError
	require.ErrorAs(t, err, &refused, "§8.4.1 pre-validates per §8.2")
	assert.Equal(t, "f1", refused.Record, "§8.2.4: the refusal names the record id")
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§8.2.4 refuses with exit code 1")
	assert.Empty(t, printed, "the run refused before it reported a payload")
}

// The control: the same record placed inside a hunk of the diff builds its
// payload and is reported as a comment. Without it the refusal above could be
// any refusal at all, and a pre-validation that refused everything would pass.
func TestAPlaceablePositionReachesThePayload(t *testing.T) {
	suggestingRound(t, suggesting("f1", 4, "\treturn fmt.Errorf(\"one\")"))

	printed, err := runPost(t, fixturePR, "--repo", fixtureSlug)

	require.NoError(t, err)
	assert.Contains(t, printed, `"id": "f1"`)
}

// longLib is the file the long-file fixture's pull request changes: the three
// lines detectedHome's base holds, then enough numbered lines that most of the
// file lies outside the one hunk the change produces.
func longLib(load ...string) string {
	lines := append([]string{"package lib", ""}, load...)
	for n := 1; n <= 27; n++ {
		lines = append(lines, fmt.Sprintf("// note %d", n))
	}
	return strings.Join(lines, "\n") + "\n"
}

// longFileHome is detectedHome's pull request over a lib.go long enough to hold
// lines no hunk reaches, behind the real diff and a gh shim answering the
// pull request.
//
// The change turns base line 3 into three head lines, so with §3.4.1's three
// lines of context the one hunk is `@@ -1,6 +1,8 @@`: head lines 1 to 8 and
// base lines 1 to 6. Head line 25 and base line 20 both exist and no hunk
// carries either.
func longFileHome(t *testing.T) state.Layout {
	t.Helper()
	dir := t.TempDir()
	write := func(body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lib.go"), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write(longLib("func Load() {}"))
	mustGit(t, dir, "add", "lib.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write(longLib("func Load() {", "\tpanic(\"one\")", "}"))
	mustGit(t, dir, "commit", "--quiet", "-am", "the change under review")
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
		`{"id":"u1","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":1,"end":8}],`+
			`"head":"`+head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// plainQuestion is suggesting's record with no suggestion, anchored on side at
// line: a comment §8.2 has nothing to say about.
func plainQuestion(side git.Side, line int) *finding.Finding {
	record := suggesting("f1", line, "")
	record.Anchor.Side = side
	return record
}

// longFileRound is longFileHome with one record stored and drafted, so the
// round holds it queued, and the path of the round's draft.md.
func longFileRound(t *testing.T, record *finding.Finding) (layout state.Layout, drafted string) {
	t.Helper()
	layout = longFileHome(t)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, state.WriteStamped(
		held, state.FileFindings, state.Stamp{Round: 1}, []*finding.Finding{record},
	))
	require.NoError(t, held.Unlock())
	_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	return layout, layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileDraft)
}

// §8.4.1 over a comment carrying no suggestion: the reviewer moves its marker in
// draft.md onto a line the head holds and the diff does not, and `cr post`
// refuses before the atomic call, naming the record, whether or not --confirm
// was given.
//
// §7.2's location row re-validates a moved marker against the tree alone, so
// this refusal is the only thing standing between the move and a review GitHub
// rejects whole. The confirmed run is the one that would have sent it, and it
// writes no posted.json, which §8.3.3 writes before the call.
func TestACommentMovedOutsideTheDiffIsRefusedBeforeTheCall(t *testing.T) {
	// The subtest names reach a shell: the gh shim's path is under t.TempDir,
	// which is named after the test, so they hold no character sh reads.
	for name, flags := range map[string][]string{"dry run": nil, "confirmed": {"--confirm"}} {
		t.Run(name, func(t *testing.T) {
			layout, drafted := longFileRound(t, plainQuestion(git.Right, 4))
			posted := layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FilePosted)
			before, err := os.ReadFile(posted)
			require.NoError(t, err)
			body, err := os.ReadFile(drafted)
			require.NoError(t, err)
			moved := markerEdit(t, string(body), "f1", `start_line="4" line="4"`, `start_line="25" line="25"`)
			require.NoError(t, os.WriteFile(drafted, []byte(moved), 0o600))

			printed, err := runPost(t, append([]string{fixturePR, "--repo", fixtureSlug}, flags...)...)

			require.Error(t, err)
			var refused *PositionError
			require.ErrorAs(t, err, &refused, "§8.4.1 holds a plain comment's position to the diff")
			assert.Equal(t, "f1", refused.Record, "the refusal names the record id")
			assert.Equal(t, [3]any{git.Right, 25, 25},
				[3]any{refused.Anchor.Side, refused.Anchor.StartLine, refused.Anchor.Line},
				"and the position the draft moved it to")
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Empty(t, printed, "the run refused before it reported a payload")
			after, err := os.ReadFile(posted)
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after), "nothing was put in front of the call")
		})
	}
}

// The control for the refusal above, and the LEFT half of it: a comment the
// diff carries on its own side reaches the payload, and one on the other side
// of the same line numbers does not.
//
// A LEFT anchor numbers merge-base lines, so base line 3 — the line the change
// rewrote — is in the hunk and base line 20 is not, while head line 4 is one of
// the lines it added. Without the placeable rows a check refusing everything
// would pass the test above.
func TestOnlyACommentTheDiffCarriesOnItsSideReachesThePayload(t *testing.T) {
	for _, tc := range []struct {
		name   string
		side   git.Side
		line   int
		places bool
	}{
		{name: "RIGHT inside the hunk", side: git.Right, line: 4, places: true},
		{name: "LEFT on the rewritten line", side: git.Left, line: 3, places: true},
		{name: "LEFT past the hunk", side: git.Left, line: 20, places: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			longFileRound(t, plainQuestion(tc.side, tc.line))

			printed, err := runPost(t, fixturePR, "--repo", fixtureSlug)

			if !tc.places {
				var refused *PositionError
				require.ErrorAs(t, err, &refused)
				assert.Equal(t, "f1", refused.Record)
				return
			}
			require.NoError(t, err)
			var report struct {
				Comments []postedComment `json:"comments"`
			}
			require.NoError(t, json.Unmarshal([]byte(printed), &report))
			assert.Equal(t, []postedComment{{ID: "f1", Kind: finding.KindQuestion}}, report.Comments)
		})
	}
}
