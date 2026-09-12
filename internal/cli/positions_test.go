package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/suggestion"
)

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
// Nothing reaches the network here and nothing can: internal/gh's write door is
// a method on a Confirmation only §8.5's gate mints, and this build mints none.
// What the test pins is the half that will still matter once it does — that the
// refusal happens while the payload is being assembled, so the atomic call is
// never reached with a position that would lose the whole round.
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

// A round whose records carry no suggestion asks for no diff, so §8.4.1 costs a
// repository read only where §8.2 has something to say. This is the guard
// indentationWarnings already makes at draft time, asserted at post time.
func TestAPostWithNoSuggestionAsksForNoDiff(t *testing.T) {
	draftedHome(t, aCitedRecord("f1"))
	redraft(t)

	printed, err := runPost(t, draftPR, "--repo", draftSlug)

	require.NoError(t, err, "no suggestion, so no hunks are read and no repository is needed")
	assert.Contains(t, printed, `"id": "f1"`)
}
