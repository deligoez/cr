package reinvention

import (
	"cmp"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/symbol"
)

// Ranking is §4.3.2's two knobs, resolved from `reinvention.min_similarity`
// and `reinvention.max_candidates`.
//
// They arrive as values rather than being read from a config here, so this
// package holds no opinion about where a setting comes from and a test can put
// the threshold anywhere it needs it. internal/config carries both keys with
// their §4.3.2 defaults, which is the one place the numbers 0.6 and 5 live.
type Ranking struct {
	// MinSimilarity is the least similarity a candidate may have and still
	// qualify.
	MinSimilarity float64
	// MaxCandidates is the most candidates one added symbol is attached.
	// Zero and below attach none.
	MaxCandidates int
}

// comparisonName is §4.3.2's comparison name: the symbol name case-folded to
// lowercase with every `_` and `-` removed.
//
// It exists because the same idea gets three spellings across one repository —
// `formatMoney`, `format_money`, `format-money` — and a distance measured over
// the raw names would rank a genuine reinvention below an unrelated symbol that
// happens to share a naming style. Folding is for the comparison only: §4.3.3
// cites the candidate at `path:line` and the reader searches for the name the
// source spells, so symbol.Decl keeps that one and never this.
func comparisonName(name string) string {
	folded := strings.ToLower(name)
	return strings.Map(func(r rune) rune {
		if r == '_' || r == '-' {
			return -1
		}
		return r
	}, folded)
}

// similarity is §4.3.2's `1 - levenshtein(a, b) / max(len(a), len(b))` over
// comparison names.
//
// Both names empty is defined as 1.0 rather than left to the division, which
// would be 0/0. Two symbols whose names fold away to nothing are as alike as
// the measure can say, and §4.3.2 fixes the value instead of letting a NaN
// reach a comparison that would then answer false to every threshold.
func similarity(a, b string) float64 {
	first, second := []rune(a), []rune(b)
	longest := max(len(first), len(second))
	if longest == 0 {
		return 1
	}
	return 1 - float64(levenshtein(first, second))/float64(longest)
}

// levenshtein is the edit distance between two names, counted in Unicode code
// points.
//
// Code points and not bytes, because §4.3.2 says so and because bytes would
// make the measure depend on the alphabet: `değer` and `deger` differ by one
// code point and by two bytes, so a byte distance would rate a Turkish
// near-match lower than the identical English one.
//
// One row is kept rather than the whole matrix. The recurrence reads only the
// row above and the cell to the left, so the matrix is never needed and a large
// index costs no more memory than a small one.
func levenshtein(first, second []rune) int {
	previous := make([]int, len(second)+1)
	for at := range previous {
		previous[at] = at
	}
	current := make([]int, len(second)+1)
	for row := 1; row <= len(first); row++ {
		current[0] = row
		for col := 1; col <= len(second); col++ {
			substitution := previous[col-1]
			if first[row-1] != second[col-1] {
				substitution++
			}
			current[col] = min(substitution, previous[col]+1, current[col-1]+1)
		}
		previous, current = current, previous
	}
	return previous[len(second)]
}

// scored is one candidate together with the similarity it qualified at. The
// score is kept only long enough to order by it: §4.3.2 makes similarity the
// first ordering key, and §4.3.4 makes the item a question, so a number cr
// attached to a symbol would read as a confidence cr never has.
type scored struct {
	decl  symbol.Decl
	score float64
}

// rank applies §4.3.2 to one added symbol's candidate pool.
//
// The parameter count is an equality and not a tolerance, and it is checked
// before the threshold because it is the cheaper of the two. §4.3.2 pairs it
// with the similarity deliberately: a name alone would offer `Find` against
// `Find` on a different arity, which is a different function with the same
// idea in its name — a question the author answers with "no" every time, and
// §4.3.4 posts every one of them.
func rank(added symbol.Decl, pool []symbol.Decl, r Ranking) []symbol.Decl {
	target := comparisonName(added.Name)
	qualified := make([]scored, 0, len(pool))
	for _, candidate := range pool {
		if candidate.Params != added.Params {
			continue
		}
		score := similarity(target, comparisonName(candidate.Name))
		if score < r.MinSimilarity {
			continue
		}
		qualified = append(qualified, scored{decl: candidate, score: score})
	}
	slices.SortStableFunc(qualified, byRank)

	limit := max(r.MaxCandidates, 0)
	if len(qualified) > limit {
		qualified = qualified[:limit]
	}
	attached := make([]symbol.Decl, 0, len(qualified))
	for _, candidate := range qualified {
		attached = append(attached, candidate.decl)
	}
	return attached
}

// byRank is §4.3.2's ordering: similarity descending, then path ascending, then
// line ascending.
//
// The tail two are not decoration. A tie on similarity is common — a repository
// with `formatMoney` and `format_money` in two files answers both at 1.0 — and
// without a total order the cut at MaxCandidates would keep a different five on
// a different run, which §2.1.1 forbids and which would make one round's
// questions disappear from the next for no reason a reader could see.
func byRank(a, b scored) int {
	return cmp.Or(
		cmp.Compare(b.score, a.score),
		cmp.Compare(a.decl.Path, b.decl.Path),
		cmp.Compare(a.decl.Line, b.decl.Line),
	)
}
