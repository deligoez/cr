package unit

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collision is one file whose merge-base side and head side name the same line
// number, 53, in two different file versions.
//
// The numbers are built rather than found. A hunk's head coordinate is its base
// coordinate plus the net lines every earlier hunk of the file added, so the
// only way a later addition's head line can reach an earlier deletion's base
// line is for a deletion further up to have pulled the head numbers back far
// enough. The first hunk removes twelve lines to do exactly that: after it the
// head runs eleven behind the base, the second hunk removes merge-base lines 53
// and 54, and the fourth adds head line 53.
//
// The first hunk is an addition on purpose. §3.4.4 partitions by side in order
// of first appearance, so this puts the RIGHT clusters ahead of the LEFT ones
// on the way in, and an ordering that leaned on the order it was handed —
// rather than on a side key — would keep them there.
const collision = `--- a/app/Money.php
+++ b/app/Money.php
@@ -1,6 +1,7 @@
 context
 context
 context
+added at head 4
 context
 context
 context
@@ -20,18 +21,6 @@
 context
 context
 context
-removed at base 23
-removed at base 24
-removed at base 25
-removed at base 26
-removed at base 27
-removed at base 28
-removed at base 29
-removed at base 30
-removed at base 31
-removed at base 32
-removed at base 33
-removed at base 34
 context
 context
 context
@@ -50,8 +39,6 @@
 context
 context
 context
-removed at base 53
-removed at base 54
 context
 context
 context
@@ -63,6 +50,7 @@
 context
 context
 context
+added at head 53
 context
 context
 context
`

// unitsOf runs the whole of §3.4.4 through §3.4.6 over a patch.
func unitsOf(t *testing.T, patch string) []Unit {
	t.Helper()
	generic := shipped(t, "generic")
	hunks, err := git.ParseHunks(patch)
	require.NoError(t, err)
	units, err := Units(Split(Clusters(hunks, &generic, nil, defaultGapLines(t)), defaultMaxLines(t)))
	require.NoError(t, err)
	return units
}

// §3.4.6 assigns ids "in ascending order of first file path then first changed
// line", and that is the one comparison in §3.4 ranging over both sides at
// once: everywhere else §3.4.4's partition has already fixed which file version
// a number is read in. Here it has not, and this file holds a LEFT unit and a
// RIGHT unit whose first changed line is 53 in two different file versions.
//
// Compared on the number alone they are tied, and the winner is whichever the
// clustering happened to hand over first — which §2.1.1 does not allow, because
// the same inputs have to give the same result and "whichever came first" is
// not a property of the inputs. The side breaks it, `LEFT` before `RIGHT` per
// §8.3.3, so the tie never has to be broken by anything else.
//
// The order is asserted whole rather than only at the tie. The RIGHT unit at
// head line 4 sorts behind both LEFT units, which is what makes this an order
// by side and not an order by line that happens to agree with one.
func TestUnitIdsOrderLeftBeforeRightAtTheSameLineNumber(t *testing.T) {
	hunks, err := git.ParseHunks(collision)
	require.NoError(t, err)
	require.Len(t, hunks, 4)
	require.Equal(t, git.Right, hunks[0].Side)
	require.Equal(t, git.Left, hunks[2].Side)
	require.Equal(t, git.Right, hunks[3].Side)
	require.Equal(t, hunks[2].Changed[0].Line, hunks[3].Changed[0].Line,
		"the fixture is only worth having if the two sides really do collide on 53")

	units := unitsOf(t, collision)
	require.Len(t, units, 4)

	assert.Equal(t, []string{"u1", "u2", "u3", "u4"},
		[]string{units[0].ID, units[1].ID, units[2].ID, units[3].ID})
	assert.Equal(t, []git.Side{git.Left, git.Left, git.Right, git.Right},
		[]git.Side{units[0].Side, units[1].Side, units[2].Side, units[3].Side},
		"§8.3.3's convention: every LEFT unit of a file before every RIGHT one")
	assert.Equal(t, 12, units[0].ChangedLines, "the first LEFT unit is the twelve-line removal")
	assert.Equal(t, 2, units[1].ChangedLines, "then the removal of merge-base 53 and 54")
	assert.Equal(t, []Range{{Start: 21, End: 26}}, units[0].HunkRanges)
	assert.Equal(t, []Range{{Start: 39, End: 44}}, units[1].HunkRanges)
	assert.Equal(t, []Range{{Start: 1, End: 7}}, units[2].HunkRanges,
		"the addition at head 4 sorts behind both LEFT units, not ahead of them")
	assert.Equal(t, []Range{{Start: 50, End: 56}}, units[3].HunkRanges)

	assert.Equal(t, units, unitsOf(t, collision),
		"§2.1.1: the same input gives the same units, ids included")
}

// orderPath is the second file the enumeration below changes. It sorts after
// moneyPath, so a path comparison that ran backwards would be visible.
const orderPath = "app/Order.php"

// fill is what one slot of the enumeration holds.
type fill int

const (
	absent fill = iota
	adds
	removes
)

// slot is a place a hunk may sit: a file, and the merge-base line its hunk
// starts at. The two slots of a file are forty lines apart, far past
// `cluster.gap_lines`, so every hunk becomes a unit of its own and the number
// of units is the number of filled slots.
type slot struct {
	path string
	base int
}

var slots = []slot{
	{moneyPath, 20}, {moneyPath, 60},
	{orderPath, 20}, {orderPath, 60},
}

// patchOf renders a valid unified diff holding one hunk per filled slot: an
// addition of one line, or a removal of one, each with three lines of context
// on either side. The head coordinate of a hunk is its merge-base coordinate
// plus what the earlier hunks of its file added or removed, so the diff is one
// a tree could actually produce and not a shape only the parser would accept.
func patchOf(filled []fill) string {
	var out strings.Builder
	for _, path := range []string{moneyPath, orderPath} {
		opened, delta := false, 0
		for i, at := range slots {
			if at.path != path || filled[i] == absent {
				continue
			}
			if !opened {
				fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", path, path)
				opened = true
			}
			head := at.base + delta
			if filled[i] == adds {
				fmt.Fprintf(&out, "@@ -%d,6 +%d,7 @@\n context\n context\n context\n+added at head %d\n context\n context\n context\n",
					at.base, head, head+3)
				delta++
				continue
			}
			fmt.Fprintf(&out, "@@ -%d,7 +%d,6 @@\n context\n context\n context\n-removed at base %d\n context\n context\n context\n",
				at.base, head, at.base+3)
			delta--
		}
	}
	return out.String()
}

// Round 8's vacuous-multi-file-unit: every branch of §3.4.4 says "same file"
// before it says anything else, so a unit carries exactly one file path, and
// §3.4.6's "its file paths" and §6.2.1's "one of that unit's file paths" range
// over a set of one.
//
// The strongest half of that is structural and not testable: Unit.Path is one
// string, sided.cluster is the only place a Cluster is built, and it takes the
// path from the partition rather than from the hunks — so a unit spanning two
// files is not a value this package can construct. What a test can add is that
// no diff drives a hunk of one file into a cluster of another, and this one
// asks it of every diff in a closed space rather than of one fixture: each of
// the two files may hold, at each of two positions, an addition, a removal, or
// nothing, which is every one of the eighty-one combinations and so every
// arrangement of files and sides the clustering can be handed.
//
// The id ordering rides along, because the same enumeration is exactly the
// space where paths and sides interleave. Every combination is asserted to
// ascend by path, then LEFT before RIGHT, then line, to number its units `u1`
// upwards, and to come out the same on a second run per §2.1.1.
func TestNoDiffPutsTwoFilesInOneUnit(t *testing.T) {
	for combination := range 81 {
		filled := make([]fill, len(slots))
		remaining, hunks := combination, 0
		for i := range filled {
			filled[i] = fill(remaining % 3)
			remaining /= 3
			if filled[i] != absent {
				hunks++
			}
		}
		if hunks == 0 {
			continue
		}

		t.Run(fmt.Sprintf("combination-%d", combination), func(t *testing.T) {
			patch := patchOf(filled)
			generic := shipped(t, "generic")
			parsed, err := git.ParseHunks(patch)
			require.NoError(t, err, patch)
			require.Len(t, parsed, hunks)

			clusters := Split(Clusters(parsed, &generic, nil, defaultGapLines(t)), defaultMaxLines(t))
			assertNoClusterMixesSides(t, clusters)
			require.Len(t, clusters, hunks, "the slots are far enough apart to cluster alone")

			units, err := Units(clusters)
			require.NoError(t, err)
			require.Len(t, units, hunks)

			for i := range units {
				assert.Equal(t, "u"+strconv.Itoa(i+1), units[i].ID)
				assert.Contains(t, []string{moneyPath, orderPath}, units[i].Path)
				require.Len(t, units[i].HunkRanges, 1)
			}
			for i := 1; i < len(units); i++ {
				assertOrdered(t, &units[i-1], &units[i])
			}

			again, err := Units(Split(Clusters(parsed, &generic, nil, defaultGapLines(t)), defaultMaxLines(t)))
			require.NoError(t, err)
			assert.Equal(t, units, again, "§2.1.1: the same inputs give the same units")
		})
	}
}

// assertOrdered holds two neighbouring units to §3.4.6's assignment order with
// the side tiebreak: ascending path, then LEFT before RIGHT, then ascending
// line. Only one of the three fires for any pair, which is what makes it an
// order and not three independent checks.
func assertOrdered(t *testing.T, prev, next *Unit) {
	t.Helper()
	switch {
	case prev.Path != next.Path:
		assert.Less(t, prev.Path, next.Path, "units ascend by file path")
	case prev.Side != next.Side:
		assert.Equal(t, git.Left, prev.Side, "inside a file, LEFT precedes RIGHT")
	default:
		assert.Less(t, prev.HunkRanges[0].Start, next.HunkRanges[0].Start,
			"inside one file and one side, units ascend by line")
	}
}
