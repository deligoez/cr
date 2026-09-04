package rule

import (
	"testing"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/unit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The round these tests attach against: one file with two RIGHT-side units at
// separate line ranges, a second file's unit, and a LEFT unit of the first file
// whose head-side insertion point falls inside the first unit's range.
//
// The units are built here rather than driven out of §3.4.4, because what is
// under test is where a hit lands and not how a cluster forms. §6.2.1's ranges
// are the whole of the input, and unit.Units is exercised by its own package.
func attachUnits() []unit.Unit {
	return []unit.Unit{
		{ID: "u1", Path: "internal/api/handler.go", Side: git.Right,
			HunkRanges: []unit.Range{{Start: 40, End: 48}}},
		{ID: "u2", Path: "internal/api/handler.go", Side: git.Right,
			HunkRanges: []unit.Range{{Start: 90, End: 92}, {Start: 120, End: 130}}},
		{ID: "u3", Path: "internal/api/client.go", Side: git.Right,
			HunkRanges: []unit.Range{{Start: 1, End: 12}}},
		{ID: "u4", Path: "internal/api/handler.go", Side: git.Left,
			HunkRanges: []unit.Range{{Start: 44, End: 44}}},
	}
}

// hitsBy reads an attachment list as a map from unit id to the matched lines,
// which is the whole of what §4.3.6 decides.
func hitsBy(attached []Attachment) map[string][]int {
	by := make(map[string][]int, len(attached))
	for _, a := range attached {
		lines := make([]int, 0, len(a.Hits))
		for _, hit := range a.Hits {
			lines = append(lines, hit.Line)
		}
		by[a.Unit] = lines
	}
	return by
}

// §4.3.6: a mechanical rule hit is attached to the unit that contains it.
//
// The second unit's two ranges are what make this more than a bounds check. A
// unit is a group of hunks, not one span, so a hit in its later hunk has to
// reach it while a hit in the gap between them reaches nothing — and an
// implementation reading only the first and last range would pass every
// single-hunk case and fail exactly here.
func TestAHitIsAttachedToTheUnitThatContainsIt(t *testing.T) {
	attached := Attach(attachUnits(), []Hit{
		{RuleID: "handle-every-error", Path: "internal/api/handler.go", Line: 44},
		{RuleID: "handle-every-error", Path: "internal/api/handler.go", Line: 125},
		{RuleID: "no-blank-error", Path: "internal/api/client.go", Line: 3},
	})

	assert.Equal(t, map[string][]int{
		"u1": {44}, "u2": {125}, "u3": {3}, "u4": {},
	}, hitsBy(attached))
}

// A hit outside every unit's hunks is attached to no unit, and above all not to
// an unrelated one.
//
// Every case here is a different way of being outside: a line of a unit's own
// file that falls in the gap between two of its hunks, a line past the last
// hunk of the file, and a line in a file this round did not change at all.
// Attaching any of them would send the role to code the match was never in, and
// §2.6.1.5 has the role confirm a hit before it reaches a draft — so a
// misplaced hit is not a harmless extra, it is a candidate the role is being
// asked to confirm about the wrong lines.
func TestAHitOutsideEveryUnitIsAttachedToNone(t *testing.T) {
	attached := Attach(attachUnits(), []Hit{
		{RuleID: "handle-every-error", Path: "internal/api/handler.go", Line: 100},
		{RuleID: "handle-every-error", Path: "internal/api/handler.go", Line: 400},
		{RuleID: "no-blank-error", Path: "internal/api/untouched.go", Line: 7},
	})

	assert.Equal(t, map[string][]int{
		"u1": {}, "u2": {}, "u3": {}, "u4": {},
	}, hitsBy(attached))
}

