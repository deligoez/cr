package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// Round 13's agent-chosen-grading-boundary through the command: a record whose
// anchor sits in u1 and names u2 is refused, naming the line and the field, and
// nothing is stored.
//
// This is the forgery the finding describes. Both units are genuine units of
// the round, so §6.1.3's check that the unit exists passes; without the binding
// the record's own hunk would be measured against u2's ranges, a citation into
// it would count as outside the record's own unit, and §6.2's `cited` row would
// be bought with no evidence beyond the diff itself.
func TestARecordAnchoredInOneUnitNamingAnotherIsRefused(t *testing.T) {
	layout := recordedHome(t)
	swapped := aRecord("f2", "u2")
	swapped["anchor"] = aRecord("f2", "u1")["anchor"]
	file := writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1"), swapped)

	_, err := runRecord(t, recordPR, file, "--repo", recordSlug)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, 2, rejected.Line)
	assert.Equal(t, "anchor", rejected.Field)
	assert.Contains(t, rejected.Problem, `unit "u2"`)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§6.1.3's refusals exit 1")
	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	assert.Empty(t, stored, "a refused file stores none of its records")
}

// The binding holds for every record of a file, and a LEFT anchor ahead of the
// forgery does not end the walk.
//
// A LEFT anchor is the one case that reads the round's diff, and gremlins found
// that nothing asserted what happens after that read succeeds: negating the
// guard on its error returned from the walk at the first LEFT record, so every
// record after it went unmeasured. The first record here is a genuine LEFT
// anchor on the base line the change rewrote, inside u1 through its hunk; the
// second is the forgery of the test above, anchored in u1 and naming u2. It is
// refused on its own line and field, which is what pins the refusal to the walk
// reaching it rather than to anything about the first record.
func TestAForgeryAfterALeftAnchorIsStillRefused(t *testing.T) {
	layout := reviewedHome(t)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":1,"end":7}],`+
			`"head":"`+meta.Head+`","round":1}`+"\n"+
			`{"id":"u2","path":"other.go","side":"RIGHT","hunk_ranges":[{"start":1,"end":1}],`+
			`"head":"`+meta.Head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())

	anchored := func(id, unit, side string, line int) map[string]any {
		record := aRecord(id, unit)
		record["anchor"] = map[string]any{
			"path": "lib.go", "side": side, "start_line": line, "line": line,
			"content_hash": "0123456789abcdef",
		}
		return record
	}
	file := writeRecordFile(t, "merged.ndjson",
		anchored("f1", "u1", "LEFT", 3), anchored("f2", "u2", "RIGHT", 4))

	_, err = runRecord(t, fixturePR, file, "--repo", fixtureSlug)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, 2, rejected.Line, "the refusal is the second record's")
	assert.Equal(t, "anchor", rejected.Field)
	assert.Contains(t, rejected.Problem, `unit "u2"`)
	assert.Empty(t, storedFindings(t, layout), "a refused file stores none of its records")
}

// Containment of a whole anchor, in head coordinates, over every shape the
// binding has to answer.
//
// The LEFT cases are the ones §6.2.1's "no side is compared" is about. A
// removed line is numbered in the merge base, and no unit range is: the anchor
// reaches head coordinates only through the hunk that removed it, so a LEFT
// anchor whose lines no hunk removed has no head location and is inside no
// unit. The straddle is the case a check of the two ends alone would pass: both
// lie in the unit, the lines between them do not.
func TestAnAnchorIsInsideItsUnitOnlyWhenEveryLineIsInHeadCoordinates(t *testing.T) {
	own := &unit.Unit{
		Path:       "lib.go",
		HunkRanges: []unit.Range{{Start: 1, End: 7}, {Start: 20, End: 26}},
	}
	// The first hunk rewrote base lines 1 to 3 into head lines 1 to 7; the
	// second removed base lines 30 to 32, leaving head lines 20 to 26 as its
	// context.
	hunks := []git.Hunk{
		{Path: "lib.go", BaseStart: 1, BaseLines: 3, HeadStart: 1, HeadLines: 7},
		{Path: "lib.go", BaseStart: 28, BaseLines: 7, HeadStart: 20, HeadLines: 7},
	}
	for name, tc := range map[string]struct {
		anchor finding.Anchor
		own    finding.Containment
		inside bool
	}{
		"a RIGHT anchor within a hunk": {finding.Anchor{Path: "lib.go", Side: git.Right, StartLine: 3, Line: 5}, own, true},
		"a RIGHT anchor past the hunk": {finding.Anchor{Path: "lib.go", Side: git.Right, StartLine: 6, Line: 8}, own, false},
		"a RIGHT anchor in another file": {
			finding.Anchor{Path: "other.go", Side: git.Right, StartLine: 3, Line: 3}, own, false,
		},
		"a RIGHT anchor straddling two hunks": {
			finding.Anchor{Path: "lib.go", Side: git.Right, StartLine: 5, Line: 22}, own, false,
		},
		"a LEFT anchor on lines a hunk removed": {
			finding.Anchor{Path: "lib.go", Side: git.Left, StartLine: 30, Line: 32}, own, true,
		},
		// The hunk's first and last merge-base lines are its own, and the line
		// after the last is the next hunk's or no hunk's: a record anchored
		// there has no head location, so it is refused rather than measured
		// against a unit it was never in.
		"a LEFT anchor on a hunk's first merge-base line": {
			finding.Anchor{Path: "lib.go", Side: git.Left, StartLine: 28, Line: 28}, own, true,
		},
		"a LEFT anchor on a hunk's last merge-base line": {
			finding.Anchor{Path: "lib.go", Side: git.Left, StartLine: 34, Line: 34}, own, true,
		},
		"a LEFT anchor one line past a hunk's merge-base lines": {
			finding.Anchor{Path: "lib.go", Side: git.Left, StartLine: 35, Line: 35}, own, false,
		},
		"a LEFT anchor running one line past a hunk's merge-base lines": {
			finding.Anchor{Path: "lib.go", Side: git.Left, StartLine: 30, Line: 35}, own, false,
		},
		"a LEFT anchor on lines no hunk removed": {
			finding.Anchor{Path: "lib.go", Side: git.Left, StartLine: 10, Line: 10}, own, false,
		},
		"a unit that did not resolve": {finding.Anchor{Path: "lib.go", Side: git.Right, StartLine: 3, Line: 3}, nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.inside, anchorInsideUnit(&tc.anchor, tc.own, hunks))
		})
	}
}
