package unit

import (
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
