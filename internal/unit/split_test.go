package unit

import (
	"fmt"
	"testing"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultMaxLines resolves `cluster.max_lines` from the built-in defaults of
// §2.7, for the reason defaultGapLines does: the number lives in one place and
// a test spelling 80 itself would keep passing after that place changed.
func defaultMaxLines(t *testing.T) int {
	t.Helper()
	cfg, err := config.Resolve(config.Sources{})
	require.NoError(t, err)
	return cfg.Int("cluster.max_lines")
}

// addedHunk is one hunk adding count lines at head line start.
//
// The fixtures below are built rather than parsed out of a diff, because what
// §3.4.5 measures is a count of changed lines and the counts it has to be
// measured at are around eighty. A patch carrying eighty additions would say
// nothing the count does not, and would hide the number the test is about
// among eighty lines of context.
func addedHunk(start, count int) git.Hunk {
	changed := make([]git.ChangedLine, count)
	for i := range changed {
		changed[i] = git.ChangedLine{
			Side: git.Right,
			Line: start + i,
			Text: fmt.Sprintf("added at head %d", start+i),
		}
	}
	return git.Hunk{
		Path: moneyPath, Side: git.Right,
		BaseStart: start - 1, BaseLines: 0,
		HeadStart: start, HeadLines: count,
		Changed: changed,
	}
}

// §3.4.5 splits a cluster over the cap "by taking its hunks in ascending start
// order and opening a new unit whenever adding the next hunk would exceed the
// limit", so it is the hunk that would not fit that closes a unit, and never
// one that would.
//
// The fixture is built at the boundary the sentence turns on: fifty and thirty
// changed lines reach exactly the cap and stay together, and the single line
// after them is the first that would exceed it. An implementation splitting at
// "reaches the limit" rather than "exceeds it" leaves three units here instead
// of two, and one that split by hunk count or by line span rather than by
// changed lines leaves a different two.
func TestASplittableClusterOpensAUnitAtTheHunkThatWouldNotFit(t *testing.T) {
	max := defaultMaxLines(t)
	fifty, thirty, one := addedHunk(100, 50), addedHunk(200, 30), addedHunk(300, 1)
	require.Equal(t, max, len(fifty.Changed)+len(thirty.Changed),
		"the first two hunks are the cap exactly, which is what must not split")

	cluster := Cluster{
		Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
		Hunks: []git.Hunk{fifty, thirty, one},
	}
	units := Split([]Cluster{cluster}, max)

	require.Len(t, units, 2)
	assert.Equal(t, Cluster{
		Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
		Hunks: []git.Hunk{fifty, thirty},
	}, units[0], "eighty changed lines is the cap and not past it")
	assert.Equal(t, Cluster{
		Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
		Hunks: []git.Hunk{one},
	}, units[1], "the hunk that would have exceeded the cap opens the next unit")

	for _, formed := range units {
		assert.False(t, formed.Oversized, "no single hunk here exceeds the cap")
	}
}
