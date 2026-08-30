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

// §3.4.4 measures adjacency between the last changed line of one hunk and the
// first changed line of the next, and "at most" makes a gap of exactly
// `cluster.gap_lines` join rather than split. The boundary is asserted from
// both sides of one real pair of hunks, so the number under test is a gap the
// diff has rather than one the test declared.
//
// A hunk with no changed line is the third case, and it is an absence rather
// than a boundary: there is no line to measure from, so it joins nothing on
// either side. git emits no such hunk — ParseHunks takes a hunk's changed
// lines from its additions, or from its removals when it adds none — but the
// type admits one, and reading it as adjacent would sweep lines nobody changed
// into somebody's unit.
func TestAdjacencyMeasuresTheGapBetweenChangedLines(t *testing.T) {
	hunks, err := git.ParseHunks(threeHunks)
	require.NoError(t, err)
	pair := hunks[:2]
	gap := 22 - 11

	assert.Len(t, Adjacency(pair, gap), 1, "a gap of exactly cluster.gap_lines joins")
	assert.Len(t, Adjacency(pair, gap-1), 2, "one line further apart than the gap splits")

	contextOnly := git.Hunk{Path: "app/Money.php", BaseStart: 12, BaseLines: 3, HeadStart: 13, HeadLines: 3}
	assert.Equal(t,
		[][]git.Hunk{{hunks[0]}, {contextOnly}, {hunks[1]}},
		Adjacency([]git.Hunk{hunks[0], contextOnly, hunks[1]}, gap),
	)
}

// A file the diff leaves alone has no hunks, and no hunks is no cluster rather
// than one empty one. The distinction is not cosmetic: §3.4.5 splits a cluster
// by its changed line count and §3.4.6 gives every unit an id and a hash of
// its changed lines, so an empty cluster reaching either would be a unit with
// nothing in it — an entry in the coverage report asking a role to review a
// change nobody made.
func TestAdjacencyGroupsNothingOutOfNoHunks(t *testing.T) {
	assert.Empty(t, Adjacency(nil, defaultGapLines))
	assert.Empty(t, Adjacency([]git.Hunk{}, defaultGapLines))
}
