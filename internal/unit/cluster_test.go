package unit

import (
	"testing"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// moneyPath is the one file the fixtures below change.
const moneyPath = "app/Money.php"

// branches is one file's diff laid out so that each branch of §3.4.4 has
// something only it can do. Head lines 11 and 40 are twenty-nine apart, far
// past the default gap, so nothing but a shared enclosing symbol can put them
// in one unit. Head lines 80 and 88 are eight apart and inside no symbol, so
// nothing but the gap can. The fallback's hunk is added in the test, because
// git does not emit one.
const branches = `--- a/app/Money.php
+++ b/app/Money.php
@@ -10,3 +10,4 @@
 context
+added at head 11
 context
 context
@@ -38,3 +39,4 @@
 context
+added at head 40
 context
 context
@@ -78,3 +79,4 @@
 context
+added at head 80
 context
 context
@@ -86,3 +87,4 @@
 context
+added at head 88
 context
 context
`

// span is one symbol over a span of head lines, both ends inclusive.
type span struct {
	name        string
	first, last int
}

// symbolsAt is a SymbolIndex over one file's symbols, standing in for §4.3.1's
// head index until the task that builds it lands. It answers about head lines
// only, which is the whole of what §4.3.1 will index.
type symbolsAt struct {
	path    string
	symbols []span
}

func (s symbolsAt) Indexed(path string) bool { return path == s.path }

func (s symbolsAt) Enclosing(path string, line int) (string, bool) {
	if path != s.path {
		return "", false
	}
	for _, symbol := range s.symbols {
		if line >= symbol.first && line <= symbol.last {
			return symbol.name, true
		}
	}
	return "", false
}

// defaultGapLines resolves `cluster.gap_lines` from the built-in defaults of
// §2.7, which is the one place the number lives. A test spelling 12 itself
// would keep passing after the setting changed underneath it.
func defaultGapLines(t *testing.T) int {
	t.Helper()
	cfg, err := config.Resolve(config.Sources{})
	require.NoError(t, err)
	return cfg.Int("cluster.gap_lines")
}

// assertNoClusterMixesSides checks §3.4.4's standing promise on whatever
// clusters it is given: a cluster's Side is the side of every hunk in it and
// of every changed line in those hunks. The check is on the lines and not only
// on the hunks because it is the lines §6.1.2 resolves and §6.2.1 contains.
func assertNoClusterMixesSides(t *testing.T, clusters []Cluster) {
	t.Helper()
	for _, cluster := range clusters {
		for _, hunk := range cluster.Hunks {
			assert.Equal(t, cluster.Side, hunk.Side, "a hunk of another side")
			assert.Equal(t, cluster.Path, hunk.Path, "a hunk of another file")
			for _, line := range hunk.Changed {
				assert.Equal(t, cluster.Side, line.Side, "a changed line of another side")
			}
		}
	}
}

// §3.4.4 orders three branches, and each one here does work no other could.
// The symbol branch joins two hunks the gap would have split; adjacency joins
// two the index encloses in no symbol; and the fallback takes the hunk that
// has no changed line to measure a gap from.
//
// The last branch is not adjacency at a gap of zero. A hunk adjacent to
// nothing is already its own cluster under adjacency, formed by a gap that was
// measured and rejected — a different fact about the unit from one no
// comparison ever placed, and §3.4.6 records which of the two it was.
func TestClusteringTakesTheThreeBranchesInOrder(t *testing.T) {
	php := shipped(t, "laravel-pest")
	require.Equal(t, "php", php.Symbols.Lang)

	parsed, err := git.ParseHunks(branches)
	require.NoError(t, err)
	require.Len(t, parsed, 4)

	// A hunk with no changed line: the type admits one, §3.4.1 keeps git
	// from emitting one, and adjacency has nothing to measure on it.
	noChangedLine := git.Hunk{
		Path: moneyPath, Side: git.Right,
		BaseStart: 95, BaseLines: 3, HeadStart: 96, HeadLines: 3,
	}
	hunks := append(append([]git.Hunk{}, parsed...), noChangedLine)

	index := symbolsAt{path: moneyPath, symbols: []span{{name: "Money::add", first: 5, last: 50}}}
	clusters := Clusters(hunks, &php, index, defaultGapLines(t))

	require.Len(t, clusters, 3)
	assert.Equal(t, Cluster{
		Path: moneyPath, Side: git.Right, Formation: BySymbol,
		Hunks: []git.Hunk{parsed[0], parsed[1]},
	}, clusters[0], "two hunks of one symbol, twenty-nine lines apart")
	assert.Equal(t, Cluster{
		Path: moneyPath, Side: git.Right, Formation: ByAdjacency,
		Hunks: []git.Hunk{parsed[2], parsed[3]},
	}, clusters[1], "two hunks inside no symbol, eight lines apart")
	assert.Equal(t, Cluster{
		Path: moneyPath, Side: git.Right, Formation: ByFallback,
		Hunks: []git.Hunk{noChangedLine},
	}, clusters[2], "a hunk no gap could be measured from")

	assertNoClusterMixesSides(t, clusters)
}

// mixedSides deletes a merge-base line and adds a head line nine lines below
// it in the same file. Nine is inside the default gap, so a clustering that
// compared the two numbers without asking which file version each names would
// put them in one unit.
const mixedSides = `--- a/app/Money.php
+++ b/app/Money.php
@@ -20,4 +20,3 @@
 context
-removed at base 21
 context
 context
@@ -30,3 +29,4 @@
 context
+added at head 30
 context
 context
`

// §3.4.4 partitions a file's hunks by side before it compares anything, and a
// unit never mixes sides. The two hunks here are nine apart on the two sides
// of one file, so the partition is the only thing keeping them apart, and the
// index names a symbol spanning both numbers, so the symbol branch would
// gather them too if it ran on the merge-base side.
//
// It must not: §4.3.1 builds the index over the head, §6.1.2 resolves a RIGHT
// anchor against the head and a LEFT one against the merge base, and §6.2.1
// evaluates containment entirely in head coordinates. A unit holding both
// sides has no coordinate space either question could be answered in, so the
// LEFT side has no index of its own and falls through to adjacency per §3.4.3.
func TestNoClusterMixesSides(t *testing.T) {
	php := shipped(t, "laravel-pest")

	hunks, err := git.ParseHunks(mixedSides)
	require.NoError(t, err)
	require.Len(t, hunks, 2)
	require.Equal(t, git.Left, hunks[0].Side)
	require.Equal(t, git.Right, hunks[1].Side)

	index := symbolsAt{path: moneyPath, symbols: []span{{name: "Money::add", first: 5, last: 50}}}
	clusters := Clusters(hunks, &php, index, defaultGapLines(t))

	require.Len(t, clusters, 2, "one cluster per side, never one across both")
	assertNoClusterMixesSides(t, clusters)

	assert.Equal(t, Cluster{
		Path: moneyPath, Side: git.Left, Formation: ByAdjacency,
		Hunks: []git.Hunk{hunks[0]},
	}, clusters[0], "the merge-base side has no index and falls through")
	assert.Equal(t, Cluster{
		Path: moneyPath, Side: git.Right, Formation: BySymbol,
		Hunks: []git.Hunk{hunks[1]},
	}, clusters[1], "the head side reads the head index")
}

// straddling adds two lines far enough apart to sit in different symbols, in
// one hunk.
const straddling = `--- a/app/Money.php
+++ b/app/Money.php
@@ -10,12 +10,14 @@
 context
+added at head 11
 context
 context
 context
 context
 context
 context
 context
 context
 context
+added at head 22
 context
 context
`

// §3.4.4's first branch groups hunks by "same enclosing symbol", and a hunk
// whose changed lines lie in two symbols has none: it shares an enclosing
// symbol with nothing, and naming it after either one would put lines that
// symbol does not contain into a unit the reader reads as that symbol's. So it
// falls to the branch below, which places it on the gap like any other hunk.
func TestAHunkStraddlingTwoSymbolsHasNoEnclosingSymbol(t *testing.T) {
	php := shipped(t, "laravel-pest")

	hunks, err := git.ParseHunks(straddling)
	require.NoError(t, err)
	require.Len(t, hunks, 1)
	require.Len(t, hunks[0].Changed, 2)

	index := symbolsAt{path: moneyPath, symbols: []span{
		{name: "Money::add", first: 5, last: 15},
		{name: "Money::subtract", first: 16, last: 30},
	}}
	clusters := Clusters(hunks, &php, index, defaultGapLines(t))

	require.Len(t, clusters, 1)
	assert.Equal(t, ByAdjacency, clusters[0].Formation)
	assert.Equal(t, hunks, clusters[0].Hunks)
}

// twoFiles changes one line in each of two files, three lines apart.
const twoFiles = `--- a/app/Money.php
+++ b/app/Money.php
@@ -10,3 +10,4 @@
 context
+added at head 11
 context
 context
--- a/app/Order.php
+++ b/app/Order.php
@@ -13,3 +13,4 @@
 context
+added at head 14
 context
 context
`

