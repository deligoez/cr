package unit

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/git"
)

// §6.2.1's containment predicate, over the three shapes a location can take
// against a unit: inside a hunk, outside every hunk, and at the head-side
// insertion point of a hunk that adds no lines.
//
// The delete-only hunk is the case the sentence exists for. Its head-side range
// is a single line — git.Hunk.HeadRange gives an insertion point Start equal to
// End — so a predicate written as a half-open interval, or one that skipped a
// range of length one as empty, would answer that a citation at the point where
// the deleted code stood lies outside the unit that deleted it. That is the
// answer §6.2's `cited` row rewards: a citation outside the record's own unit
// buys the grade, and the record would be graded on a location it is already
// about.
//
// The unit's own Side is RIGHT in one case and LEFT in another, and the answers
// do not move with it. §6.2.1 says "no side is compared", and a unit whose
// changed lines are numbered in the merge base still answers this question in
// head coordinates, because that is the only coordinate system the ranges are
// recorded in.
func TestContainmentIsDecidedInHeadCoordinates(t *testing.T) {
	changed := Unit{
		Path: "internal/api/handler.go", Side: git.Right,
		HunkRanges: []Range{{Start: 40, End: 44}, {Start: 90, End: 92}},
	}
	deleted := Unit{
		Path: "internal/api/legacy.go", Side: git.Left,
		// A hunk that adds no lines: §3.4.6 records its head-side
		// insertion point, which is one line wide.
		HunkRanges: []Range{{Start: 17, End: 17}},
	}

	for name, tc := range map[string]struct {
		unit   Unit
		path   string
		line   int
		inside bool
	}{
		"the first line of a hunk is inside":  {changed, "internal/api/handler.go", 40, true},
		"a line within a hunk is inside":      {changed, "internal/api/handler.go", 42, true},
		"the last line of a hunk is inside":   {changed, "internal/api/handler.go", 44, true},
		"the line above a hunk is outside":    {changed, "internal/api/handler.go", 39, false},
		"the line below a hunk is outside":    {changed, "internal/api/handler.go", 45, false},
		"a later hunk of the unit is inside":  {changed, "internal/api/handler.go", 91, true},
		"the gap between two hunks is out":    {changed, "internal/api/handler.go", 60, false},
		"another file is outside":             {changed, "internal/api/other.go", 42, false},
		"a delete-only insertion point is in": {deleted, "internal/api/legacy.go", 17, true},
		"one line above that point is out":    {deleted, "internal/api/legacy.go", 16, false},
		"one line below that point is out":    {deleted, "internal/api/legacy.go", 18, false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.inside, tc.unit.Contains(tc.path, tc.line))
		})
	}
}

// A unit with no hunks contains nothing, and so does the zero unit.
//
// It is not a shape §3.4.5 leaves behind — a unit is formed out of hunks — but
// it is the shape a lookup that found nothing hands back, and §6.2's `cited`
// row rewards the wrong answer here: everything outside the record's own unit
// counts toward the grade, so a predicate that answered "inside" for a unit
// that is not there would be the safe direction and this one is not.
func TestAUnitWithNoHunksContainsNothing(t *testing.T) {
	hunkless, zero := Unit{Path: "internal/api/handler.go"}, Unit{}
	assert.False(t, hunkless.Contains("internal/api/handler.go", 1))
	assert.False(t, zero.Contains("", 0), "and the zero unit holds no location either")
}
