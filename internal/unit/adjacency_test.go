package unit

import (
	"testing"

	"github.com/deligoez/cr/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// threeHunks is one file with three added lines, at head lines 11, 22 and 53.
// The first two sit eleven lines apart and the third thirty-one, so the
// default `cluster.gap_lines` of 12 falls between them and the diff has an
// answer to give: two groups, not one and not three.
const threeHunks = `--- a/app/Money.php
+++ b/app/Money.php
@@ -10,3 +10,4 @@
 context
+added at head 11
 context
 context
@@ -20,3 +21,4 @@
 context
+added at head 22
 context
 context
@@ -50,3 +52,4 @@
 context
+added at head 53
 context
 context
`

// defaultGapLines is §3.4.4's default for `cluster.gap_lines`. The setting
// itself belongs to the task that implements §3.4.4; what this task needs is
// the number the fallthrough is exercised at.
const defaultGapLines = 12

// §3.4.3 requires clustering to fall through to adjacency without reporting an
// error when a symbol is not detectable, and the shipped generic profile is
// the case that reaches it in practice: it declares no symbols.lang, so no
// index cr could build changes the answer.
//
// The assertion is that clustering still happened. A fallthrough that returned
// no units, or that surfaced the missing language, would satisfy "no error"
// and still be wrong — §4.3.1 is the half that must speak up about the same
// absent field, and this half must not, because it still answers the question
// it was asked. So the test pins the groups adjacency produces and pins that
// nothing along the way had an error to return.
func TestTheGenericProfileClustersByAdjacencyWithoutError(t *testing.T) {
	generic := shipped(t, "generic")
	require.Empty(t, generic.Symbols.Lang)

	hunks, err := git.ParseHunks(threeHunks)
	require.NoError(t, err)
	require.Len(t, hunks, 3)

	// An index covering every file in the diff, so the false answer is
	// the absent symbols.lang and nothing else.
	require.False(t, Detectable(&generic, indexOf{"app/Money.php"}, "app/Money.php"))

	groups := Adjacency(hunks, defaultGapLines)

	assert.Equal(t, [][]git.Hunk{{hunks[0], hunks[1]}, {hunks[2]}}, groups)
}
