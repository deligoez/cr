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
	maxLines := defaultMaxLines(t)
	fifty, thirty, one := addedHunk(100, 50), addedHunk(200, 30), addedHunk(300, 1)
	require.Equal(t, maxLines, len(fifty.Changed)+len(thirty.Changed),
		"the first two hunks are the cap exactly, which is what must not split")

	cluster := Cluster{
		Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
		Hunks: []git.Hunk{fifty, thirty, one},
	}
	units := Split([]Cluster{cluster}, maxLines)

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

// §3.4.5 names one case where the cap does not bind: "a single hunk that alone
// exceeds the limit MUST become one unit, MUST be flagged `oversized`". There
// is nothing below a hunk to split, and a unit cut at a line the diff never
// marked would carry a hunk range no hunk has.
//
// The flag is on that case and on nothing else, so the fixture puts an
// ordinary hunk on either side of the oversized one — each of which the cap
// does bind, and neither of which is flagged — and adds a second cluster whose
// one hunk sits at exactly the cap. That hunk is the case an implementation
// flagging at "reaches the limit" would call oversized, and it is the same
// boundary from the other side: a hunk at the cap is a plain unit.
//
// The third cluster is a lone oversized hunk, and it is the one arrangement
// where the split has nothing open on either side of the exception: no unit is
// waiting to be closed when it arrives, and none is left waiting when it
// leaves. Both of the split's "is a unit open?" tests are only tests here —
// everywhere else the answer is implied by a count — so a split that closed a
// unit it had never opened would emit a hunkless unit at exactly this point and
// nowhere else. Mutation testing is what named it: both boundaries survived
// until this cluster existed.
func TestAHunkOverTheCapAloneIsOneOversizedUnit(t *testing.T) {
	maxLines := defaultMaxLines(t)
	before, over, after := addedHunk(100, 10), addedHunk(200, maxLines+1), addedHunk(400, 10)
	atCap, alone := addedHunk(600, maxLines), addedHunk(800, maxLines+1)

	units := Split([]Cluster{
		{
			Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
			Hunks: []git.Hunk{before, over, after},
		},
		{
			Path: moneyPath, Side: git.Right, Formation: BySymbol,
			Hunks: []git.Hunk{atCap},
		},
		{
			Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
			Hunks: []git.Hunk{alone},
		},
	}, maxLines)

	require.Len(t, units, 5)
	assert.Equal(t, Cluster{
		Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
		Hunks: []git.Hunk{before},
	}, units[0], "the hunk before it is closed by the one that would not fit")
	assert.Equal(t, Cluster{
		Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
		Hunks: []git.Hunk{over}, Oversized: true,
	}, units[1], "the hunk the cap cannot bind is one unit, flagged")
	assert.Equal(t, Cluster{
		Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
		Hunks: []git.Hunk{after},
	}, units[2], "what follows it starts a unit of its own")
	assert.Equal(t, Cluster{
		Path: moneyPath, Side: git.Right, Formation: BySymbol,
		Hunks: []git.Hunk{atCap},
	}, units[3], "a hunk at exactly the cap is inside it, and the formation survives the split")
	assert.Equal(t, Cluster{
		Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
		Hunks: []git.Hunk{alone}, Oversized: true,
	}, units[4], "a cluster that is only an oversized hunk is that one unit and no other")
}

// §3.4.5's default for `cluster.max_lines` is 80, and §2.7 makes the built-in
// defaults the lowest layer rather than a constant somewhere in the splitting
// code. Both halves are asserted, as they are for the gap: the number, and
// that a layer above the default can actually reach the key — which a name
// absent from the settings table could not, since an unknown key configures
// nothing.
func TestTheMaxLinesDefaultComesFromTheSettingsTable(t *testing.T) {
	assert.Equal(t, 80, defaultMaxLines(t))

	cfg, err := config.Resolve(config.Sources{Flags: map[string]any{"cluster.max_lines": 40}})
	require.NoError(t, err)
	assert.Equal(t, 40, cfg.Int("cluster.max_lines"))
}
