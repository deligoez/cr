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
//
// The fallback unit takes `u1`. §3.4.6 assigns ids by first changed line and
// that unit has no changed line, so the ordering answers 0 for it and puts it
// ahead of the file. Nothing a diff produces lands there — git emits no hunk
// without a changed line, which is why the fixture has to build one by hand —
// and any other answer would be an invented position rather than a smaller one.
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
			HunkRanges:   []Range{{Start: 96, End: 98}},
			ChangedLines: 0, Formation: ByFallback,
		},
		{
			ID: "u2", Path: moneyPath, Side: git.Right,
			HunkRanges:   []Range{{Start: 10, End: 13}, {Start: 39, End: 42}},
			ChangedLines: 2, Formation: BySymbol,
		},
		{
			ID: "u3", Path: moneyPath, Side: git.Right,
			HunkRanges:   []Range{{Start: 79, End: 82}, {Start: 87, End: 90}},
			ChangedLines: 2, Formation: ByAdjacency,
		},
	}, units)
}

// changedHunk is one hunk whose changed lines carry the given texts, starting
// at head line start. The texts are the point of the fixture below, so they are
// written out rather than generated.
func changedHunk(start int, texts ...string) git.Hunk {
	changed := make([]git.ChangedLine, len(texts))
	for i, body := range texts {
		changed[i] = git.ChangedLine{Side: git.Right, Line: start + i, Text: body}
	}
	return git.Hunk{
		Path: moneyPath, Side: git.Right,
		BaseStart: start - 1, BaseLines: 0,
		HeadStart: start, HeadLines: len(texts),
		Changed: changed,
	}
}

// pinnedUnitHash is §3.4.6's unit hash for the two hunks below: SHA-256 over
// their changed lines joined by LF and normalised per §1.4, lowercase hex,
// first sixteen characters. It is a literal on purpose — an expected value
// computed the way the code computes it agrees with any implementation,
// including a wrong one.
const pinnedUnitHash = "3040505c498fab51"

// rawUnitHash is the same sixteen-character lowercase hex string a unit hash
// taken over the raw lines would be, and it is wrong. Every shape assertion
// §1.4 makes passes it, which is why the value and not the shape is what the
// test holds the implementation to.
const rawUnitHash = "d1537a8b7cf8801f"

// §3.4.6 fixes the unit hash as "the normalised hash per §1.4 of the unit's
// changed lines taken file by file in ascending path order and, within a file,
// ascending line order, joined by LF". Three things can go wrong with that
// sentence and each has an assertion here.
//
// The pre-image can be built from the wrong text: the lines below are indented
// with tabs, hold a double space and a trailing tab, so a hash taken before
// §1.4's steps ran lands on rawUnitHash — a perfectly well-formed value that
// nothing downstream would reject.
//
// It can stop early: the line that changes in the second half of the test sits
// in the *last* hunk, so a pre-image assembled out of the first hunk alone, or
// hashed per hunk, keeps the old value.
//
// And it can be built from too much: two units whose lines differ only in the
// whitespace §1.4 removes are the same text and must hash the same, which is
// the property every reader of the hash relies on — §6.4's duplicate
// suppression and §7.4.1's waiver key both read equal hashes as "the same
// code".
func TestTheUnitHashIsSection14sValueOverTheUnitsChangedLines(t *testing.T) {
	opening := changedHunk(11, "\tif ($discount) {", "\t\t$total  -= $discount;\t")
	closing := changedHunk(30, "\t\treturn  $total;\t")

	units, err := Units([]Cluster{{
		Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
		Hunks: []git.Hunk{opening, closing},
	}})
	require.NoError(t, err)
	require.Len(t, units, 1)
	assert.Equal(t, pinnedUnitHash, units[0].Hash,
		"§1.4's value over the unit's changed lines joined by LF")
	assert.NotEqual(t, rawUnitHash, units[0].Hash,
		"and not over the raw lines, whose own §1.4-shaped hash is this and is wrong")

	edited, err := Units([]Cluster{{
		Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
		Hunks: []git.Hunk{opening, changedHunk(30, "\t\treturn  $total + 1;\t")},
	}})
	require.NoError(t, err)
	require.Len(t, edited, 1)
	assert.NotEqual(t, units[0].Hash, edited[0].Hash,
		"a changed line changed, in the last hunk, so the unit hash moves")

	reindented, err := Units([]Cluster{{
		Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
		Hunks: []git.Hunk{
			changedHunk(11, "  if ($discount) {", "    $total -=   $discount;   "),
			changedHunk(30, " \t return   $total; "),
		},
	}})
	require.NoError(t, err)
	require.Len(t, reindented, 1)
	assert.Equal(t, units[0].Hash, reindented[0].Hash,
		"§1.4 removes exactly this whitespace, so the two units are the same text")
}
