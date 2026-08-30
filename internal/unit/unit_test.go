package unit

import (
	"testing"

	"github.com/deligoez/cr/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §3.4.6 enumerates what a unit records: an id of the form `u<n>`, its file
// paths, its hunk ranges, its changed line count, the unit hash, and which
// branch of §3.4.4 formed it. Every one of them is asserted here, over the
// fixture that gives all three formations, so a record missing one or filling
// one from the wrong place fails on the field and not on a count.
//
// The hunk ranges are head-side on purpose, because §6.2.1 evaluates
// containment "entirely in head coordinates" against "the head-side range of
// one of its hunks". A record keeping the merge-base numbers would look right
// on this fixture, whose two sides agree nowhere, and would put every §6.2.1
// answer one file version out.
//
// The hash is lifted out of the comparison rather than pinned twice: its value
// is a contract with §1.4 and is fixed by the test below, and restating it here
// would be a second place to keep in agreement with it. What is asserted here
// is that every unit has one and that they are not all the same string.
func TestAUnitRecordsTheFieldsSection346Names(t *testing.T) {
	php := shipped(t, "laravel-pest")

	parsed, err := git.ParseHunks(branches)
	require.NoError(t, err)
	require.Len(t, parsed, 4)

	noChangedLine := git.Hunk{
		Path: moneyPath, Side: git.Right,
		BaseStart: 95, BaseLines: 3, HeadStart: 96, HeadLines: 3,
	}
	hunks := append(append([]git.Hunk{}, parsed...), noChangedLine)

	index := symbolsAt{path: moneyPath, symbols: []span{{name: "Money::add", first: 5, last: 50}}}
	clusters := Split(Clusters(hunks, &php, index, defaultGapLines(t)), defaultMaxLines(t))
	units, err := Units(clusters)
	require.NoError(t, err)
	require.Len(t, units, 3)

	hashes := make([]string, len(units))
	for i := range units {
		hashes[i] = units[i].Hash
		assert.Regexp(t, `^[0-9a-f]{16}$`, hashes[i], "§1.4 says sixteen lowercase hex characters")
		units[i].Hash = ""
	}
	assert.NotEqual(t, hashes[0], hashes[1], "two units of different lines hash differently")

	assert.Equal(t, []Unit{
		{
			ID: "u1", Path: moneyPath, Side: git.Right,
			HunkRanges:   []Range{{Start: 10, End: 13}, {Start: 39, End: 42}},
			ChangedLines: 2, Formation: BySymbol,
		},
		{
			ID: "u2", Path: moneyPath, Side: git.Right,
			HunkRanges:   []Range{{Start: 79, End: 82}, {Start: 87, End: 90}},
			ChangedLines: 2, Formation: ByAdjacency,
		},
		{
			ID: "u3", Path: moneyPath, Side: git.Right,
			HunkRanges:   []Range{{Start: 96, End: 98}},
			ChangedLines: 0, Formation: ByFallback,
		},
	}, units)
}
