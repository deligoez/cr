package unit

import (
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/profile"
)

// Formation names the branch of §3.4.4 that formed a cluster. §3.4.6 records
// it on the unit, so a reader is told why a unit holds the hunks it holds and
// not only which ones they are.
type Formation string

const (
	// BySymbol grouped the hunks of one enclosing symbol.
	BySymbol Formation = "symbol"
	// ByAdjacency grouped hunks the gap between their changed lines placed.
	ByAdjacency Formation = "adjacency"
	// ByFallback took one hunk neither branch above could place.
	ByFallback Formation = "fallback"
)

// Cluster is one group of hunks §3.4.4 formed: what §3.4.5 splits and §3.4.6
// turns into a unit.
//
// A cluster carries one path and one Side and never a hunk from another file
// or another side. Both grouping branches of §3.4.4 are same-file, and the
// side comes from the partition §3.4.4 makes before any branch runs. That
// partition is not tidiness: §6.1.2 resolves a RIGHT anchor against the head
// and a LEFT one against the merge base, and §6.2.1 evaluates containment
// entirely in head coordinates, so a cluster holding both sides would have no
// coordinate space to be read in and a finding inside it would point at the
// wrong version of the file.
type Cluster struct {
	// Path is the file every hunk of the cluster belongs to.
	Path string
	// Side is the side every changed line of the cluster is numbered on.
	Side git.Side
	// Formation is the branch of §3.4.4 that formed the cluster.
	Formation Formation
	// Hunks are the cluster's hunks, in the order the diff gave them.
	Hunks []git.Hunk
	// Oversized marks §3.4.5's one exception: a cluster holding a single
	// hunk whose own changed lines already exceed `cluster.max_lines`.
	// Split sets it; §3.4.4 forms no cluster carrying it.
	Oversized bool
}

// Clusters forms §3.4.4's clusters out of one round's hunks.
//
// It partitions first, by file and by side, and then takes the branches in
// order inside each partition: the hunks sharing an enclosing symbol where
// §3.4.3 makes one detectable, then adjacency over what is left, then one
// cluster per hunk for what adjacency has nothing to measure.
//
// gapLines is `cluster.gap_lines`. index is nil until §4.3.1's head index
// exists — the state of every run today — and Detectable reads that as a no
// rather than as a missing answer, so the symbol branch simply does not fire
// and §3.4.3's fallthrough is the whole behaviour.
func Clusters(hunks []git.Hunk, p *profile.Profile, index SymbolIndex, gapLines int) []Cluster {
	clusters := make([]Cluster, 0, len(hunks))
	for _, part := range partition(hunks) {
		clusters = append(clusters, part.clusters(p, index, gapLines)...)
	}
	return clusters
}

// sided is one file's hunks on one side: the partition §3.4.4 makes before it
// compares any line number to any other.
type sided struct {
	path  string
	side  git.Side
	hunks []git.Hunk
}

// partition splits hunks by file and by side, keeping each partition's hunks
// in the order the diff gave them — which for one file's hunks is ascending on
// whichever side they are numbered on. Partitions come back in order of first
// appearance, so the same diff always yields the same clusters in the same
// order without a comparator having to invent one.
func partition(hunks []git.Hunk) []sided {
	type key struct {
		path string
		side git.Side
	}
	parts := make([]sided, 0, len(hunks))
	at := make(map[key]int, len(hunks))
	for _, h := range hunks {
		k := key{path: h.Path, side: h.Side}
		i, seen := at[k]
		if !seen {
			i = len(parts)
			at[k] = i
			parts = append(parts, sided{path: h.Path, side: h.Side})
		}
		parts[i].hunks = append(parts[i].hunks, h)
	}
	return parts
}

// clusters takes §3.4.4's three branches in order over one partition.
func (s sided) clusters(p *profile.Profile, index SymbolIndex, gapLines int) []Cluster {
	clusters, rest := s.bySymbol(p, index)
	for _, group := range Adjacency(rest, gapLines) {
		clusters = append(clusters, s.cluster(placement(group), group))
	}
	return clusters
}

// bySymbol takes §3.4.4's first branch, returning the clusters whose hunks
// share an enclosing symbol and the hunks left over for the branches below it.
//
// The whole partition is left over when §3.4.3 makes no symbol detectable, and
// so is a hunk the index encloses in no single symbol. A hunk straddling two
// symbols shares an enclosing symbol with nothing, and naming it after either
// one would put lines that symbol does not contain into a unit the reader
// reads as that symbol's.
func (s sided) bySymbol(p *profile.Profile, index SymbolIndex) (clusters []Cluster, rest []git.Hunk) {
	clusters, rest = make([]Cluster, 0), make([]git.Hunk, 0, len(s.hunks))
	if s.side != git.Right || !Detectable(p, index, s.path) {
		return clusters, append(rest, s.hunks...)
	}
	symbols := make([]string, 0, len(s.hunks))
	grouped := make(map[string][]git.Hunk, len(s.hunks))
	for i := range s.hunks {
		symbol, ok := enclosing(index, s.path, &s.hunks[i])
		if !ok {
			rest = append(rest, s.hunks[i])
			continue
		}
		if _, seen := grouped[symbol]; !seen {
			symbols = append(symbols, symbol)
		}
		grouped[symbol] = append(grouped[symbol], s.hunks[i])
	}
	for _, symbol := range symbols {
		clusters = append(clusters, s.cluster(BySymbol, grouped[symbol]))
	}
	return clusters, rest
}

// enclosing names the symbol enclosing every changed line of h, and reports
// false when h has no changed line or when its changed lines are not all
// inside one symbol.
func enclosing(index SymbolIndex, path string, h *git.Hunk) (string, bool) {
	if len(h.Changed) == 0 {
		return "", false
	}
	symbol, ok := index.Enclosing(path, h.Changed[0].Line)
	if !ok {
		return "", false
	}
	for _, line := range h.Changed[1:] {
		if next, found := index.Enclosing(path, line.Line); !found || next != symbol {
			return "", false
		}
	}
	return symbol, true
}

// placement names the branch that placed a group adjacency returned.
//
// §3.4.4's last branch is not adjacency at a gap of zero: a hunk adjacent to
// nothing is already its own cluster, formed by a gap adjacency measured and
// rejected. What is left for the last branch is a hunk with no changed line at
// all, which has no gap to measure from and so was placed by no comparison.
// git emits no such hunk — §3.4.1 leaves a hunk that adds nothing described by
// its removals — so this is the branch for a hunk that reached clustering from
// somewhere other than the parser.
func placement(group []git.Hunk) Formation {
	for i := range group {
		if _, _, ok := changedSpan(&group[i]); ok {
			return ByAdjacency
		}
	}
	return ByFallback
}

// cluster records one cluster. It is the only place a Cluster is built, and it
// takes the path and the side from the partition rather than from the hunks,
// so "a unit never mixes sides" is a shape this package cannot express instead
// of a property its comparisons happen to preserve.
func (s sided) cluster(formation Formation, hunks []git.Hunk) Cluster {
	return Cluster{Path: s.path, Side: s.side, Formation: formation, Hunks: hunks}
}
