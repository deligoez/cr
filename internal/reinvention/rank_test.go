package reinvention

import (
	"encoding/json"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/symbol"
)

// defaultRanking is §4.3.2's ranking as a round with no configuration of its
// own resolves it, read through the settings table rather than restated here,
// so a default that moved in internal/config moves every case that relies on it.
func defaultRanking(t *testing.T) Ranking {
	t.Helper()
	cfg, err := config.Resolve(config.Sources{})
	require.NoError(t, err)
	return Ranking{
		MinSimilarity: cfg.Float("reinvention.min_similarity"),
		MaxCandidates: cfg.Int("reinvention.max_candidates"),
	}
}

// withParams is a declaration with an explicit declared parameter count, for
// the cases where §4.3.2's equality is the thing under test.
func withParams(path string, line int, name string, params int) symbol.Decl {
	return symbol.Decl{Path: path, Line: line, Name: name, Kind: symbol.Function, Params: params}
}

// §4.3.2 fixes both knobs' defaults: 0.6 and 5. They live in the settings table
// and nowhere else, and a layer above the defaults reaches them, which a key the
// table did not carry could not.
func TestTheRankingDefaultsComeFromTheSettingsTable(t *testing.T) {
	assert.Equal(t, Ranking{MinSimilarity: 0.6, MaxCandidates: 5}, defaultRanking(t))

	cfg, err := config.Resolve(config.Sources{Flags: map[string]any{
		"reinvention.min_similarity": 0.8,
		"reinvention.max_candidates": 2,
	}})
	require.NoError(t, err)
	assert.InDelta(t, 0.8, cfg.Float("reinvention.min_similarity"), 0)
	assert.Equal(t, 2, cfg.Int("reinvention.max_candidates"))
}

// §4.3.2's comparison name: the symbol name case-folded to lowercase with every
// `_` and `-` removed. The three spellings one idea gets across a repository
// fold to one name, and nothing else is removed — a digit or a letter from
// another script is part of the name.
func TestTheComparisonNameFoldsCaseAndDropsUnderscoresAndHyphens(t *testing.T) {
	for _, spelling := range []string{"formatMoney", "format_money", "format-money", "FORMAT_MONEY", "Format-_Money"} {
		assert.Equal(t, "formatmoney", comparisonName(spelling), spelling)
	}
	assert.Equal(t, "değerhesapla2", comparisonName("Değer_Hesapla2"))
	assert.Empty(t, comparisonName("__-_"))
}

// §4.3.2's similarity is `1 - levenshtein(a, b) / max(len(a), len(b))`,
// counted in Unicode code points and defined as 1.0 when both names are empty.
func TestSimilarityIsOneMinusTheNormalisedEditDistance(t *testing.T) {
	for name, tc := range map[string]struct {
		a, b string
		want float64
	}{
		"identical names":              {"formatmoney", "formatmoney", 1},
		"the textbook pair":            {"kitten", "sitting", 1 - 3.0/7},
		"one insertion":                {"formatmoney", "formatmoneys", 1 - 1.0/12},
		"nothing in common":            {"abc", "xyz", 0},
		"one side empty":               {"", "abc", 0},
		"both empty is defined as one": {"", "", 1},
		// One code point apart, and two bytes apart: a byte distance
		// would rate this pair 1 - 2/6 instead of 1 - 1/5.
		"counted in code points": {"değer", "deger", 1 - 1.0/5},
	} {
		t.Run(name, func(t *testing.T) {
			assert.InDelta(t, tc.want, similarity(tc.a, tc.b), 1e-12)
			assert.InDelta(t, tc.want, similarity(tc.b, tc.a), 1e-12, "the measure is symmetric")
		})
	}
}

// The distance itself, at the shapes a one-row implementation gets wrong:
// either side empty, a transposition, and a substitution in the last column.
func TestTheEditDistanceCountsInsertionsDeletionsAndSubstitutions(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"abc", "abd", 1},
		{"ab", "ba", 2},
		{"flaw", "lawn", 2},
		{"kitten", "sitting", 3},
	} {
		assert.Equal(t, tc.want, levenshtein([]rune(tc.a), []rune(tc.b)), "%q to %q", tc.a, tc.b)
	}
}

// A candidate qualifies only when its similarity is at least the threshold and
// its declared parameter count equals the added symbol's. Both halves are
// needed: a name alone offers the same idea at a different arity, and an arity
// alone offers every one-argument function in the repository.
func TestACandidateQualifiesOnBothTheThresholdAndTheParameterCount(t *testing.T) {
	added := withParams("app/new.go", 3, "formatMoney", 1)
	pool := []symbol.Decl{
		withParams("app/a.go", 1, "format_money", 1), // same name, same arity
		withParams("app/a.go", 2, "formatMoney", 2),  // same name, other arity
		withParams("app/a.go", 3, "parseQueue", 1),   // same arity, other name
		withParams("app/a.go", 4, "abcdxfghijk", 1),  // the boundary pair below
		withParams("app/a.go", 5, "abcdxfghijz", 1),
	}
	// The last two share nothing with "formatmoney", so under the default
	// threshold they fall out; they only measure the boundary once the
	// added symbol is their neighbour.
	ranked := rank(added, pool, defaultRanking(t))
	assert.Equal(t, []string{"format_money"}, names(ranked))

	boundary := withParams("app/new.go", 9, "abcdefghijk", 1)
	// 1 - 1/11 and 1 - 2/11, against a threshold placed exactly on the
	// first: "at least" includes the boundary and nothing below it.
	atThreshold := Ranking{MinSimilarity: similarity("abcdefghijk", "abcdxfghijk"), MaxCandidates: 5}
	assert.Equal(t, []string{"abcdxfghijk"}, names(rank(boundary, pool, atThreshold)))
}

// The declared parameter count of a class is its constructor's, or zero when it
// declares none, and the index is where that is settled. This follows it end to
// end: a PHP class the diff adds is offered the pre-existing class whose
// constructor matches, and not the one whose name is closer but which declares
// no constructor at all.
func TestAClassQualifiesOnItsConstructorsParameterCount(t *testing.T) {
	index, built := symbol.Build("php", []symbol.File{
		phpFile("app/Orders/RefundOrder.php",
			"<?php", "class RefundOrder", "{",
			"    public function __construct(Gateway $g, Clock $c)", "    {", "    }", "}"),
		phpFile("app/Orders/RefundOrders.php", "<?php", "class RefundOrders", "{", "}"),
		phpFile("app/Billing/RefundsOrder.php",
			"<?php", "class RefundsOrder", "{",
			"    public function __construct(Gateway $g, Clock $c)", "    {", "    }", "}"),
	})
	require.True(t, built)

	attachments := Attach(indexable(), index,
		[]git.Hunk{addedAt("app/Billing/RefundsOrder.php", 1, 2, 3, 4, 5, 6, 7)}, defaultRanking(t))

	class := attachedTo(t, attachments, "RefundsOrder")
	assert.Equal(t, 2, class.Added.Params, "its constructor declares two parameters")
	assert.Equal(t, []string{"RefundOrder"}, names(class.Candidates),
		"RefundOrders is as close by name, but declares no constructor, so its count is zero")
}

// Qualifying candidates are ordered by similarity descending, then path
// ascending, then line ascending, and at most `reinvention.max_candidates` are
// attached. The ties are the case that matters: without the tail of the order
// the cut would keep a different few on a different run.
func TestCandidatesAreOrderedBySimilarityThenPathThenLineAndCut(t *testing.T) {
	added := withParams("app/new.go", 3, "formatMoney", 1)
	pool := []symbol.Decl{
		withParams("b/x.go", 5, "format_money", 1), // 1.0
		withParams("a/z.go", 1, "formatMoneys", 1), // 1 - 1/12
		withParams("a/y.go", 9, "formatMoney", 1),  // 1.0
		withParams("a/y.go", 2, "FormatMoney", 1),  // 1.0
	}

	ranked := rank(added, pool, Ranking{MinSimilarity: 0.6, MaxCandidates: 5})
	assert.Equal(t, []string{"a/y.go:2", "a/y.go:9", "b/x.go:5", "a/z.go:1"}, citations(ranked))

	cut := rank(added, pool, Ranking{MinSimilarity: 0.6, MaxCandidates: 3})
	assert.Equal(t, []string{"a/y.go:2", "a/y.go:9", "b/x.go:5"}, citations(cut),
		"the cut keeps the head of the order, not an arbitrary three")

	reversed := slices.Clone(pool)
	slices.Reverse(reversed)
	assert.Equal(t, citations(cut), citations(rank(added, reversed, Ranking{MinSimilarity: 0.6, MaxCandidates: 3})),
		"§2.1.1: the order the pool arrived in decides nothing")
}

// "At most" includes none. A cap of zero, or a nonsensical negative one,
// attaches nothing rather than everything — the direction that cannot flood a
// prompt with every symbol in the repository.
func TestACapOfZeroOrBelowAttachesNothing(t *testing.T) {
	added := withParams("app/new.go", 3, "formatMoney", 1)
	pool := []symbol.Decl{withParams("app/a.go", 1, "formatMoney", 1)}

	for _, limit := range []int{0, -1} {
		ranked := rank(added, pool, Ranking{MinSimilarity: 0, MaxCandidates: limit})
		assert.Empty(t, ranked, "max_candidates %d", limit)
		assert.NotNil(t, ranked, "§12: an empty list serialises as [], never null")
	}
}

// Semantic equivalence is the agent's judgement, not cr's. What cr emits for an
// added symbol is the symbol and its candidates — where each one is and what it
// declares — and no field anywhere that says whether it is a reinvention, how
// likely that is, or how similar cr measured it to be. A similarity score beside
// a candidate would read as a confidence, and §4.3.4 posts the item to a
// colleague.
func TestCrEmitsCandidatesAndNoVerdict(t *testing.T) {
	index := head(
		withParams("app/existing.go", 10, "formatMoney", 1),
		withParams("app/new.go", 3, "format_money", 1),
	)

	attachments := Attach(indexable(), index, []git.Hunk{addedAt("app/new.go", 3)}, defaultRanking(t))
	require.Len(t, attachments.Attached, 1)
	require.Len(t, attachments.Attached[0].Candidates, 1, "cr did emit the candidate")

	encoded, err := json.Marshal(attachments.Attached[0])
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.ElementsMatch(t, []string{"added", "candidates"}, keysOf(decoded),
		"an attachment is the added symbol and its candidates, and nothing that judges them")

	candidates, ok := decoded["candidates"].([]any)
	require.True(t, ok)
	candidate, ok := candidates[0].(map[string]any)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"path", "line", "name", "kind", "params"}, keysOf(candidate),
		"a candidate is a location and a declaration, with no score and no verdict")
}

// phpFile is one PHP head file, written line by line.
func phpFile(path string, lines ...string) symbol.File {
	return symbol.File{Path: path, Lines: lines}
}

// attachedTo finds the attachment for the added symbol of that name.
func attachedTo(t *testing.T, attachments Attachments, name string) Attachment {
	t.Helper()
	for _, attachment := range attachments.Attached {
		if attachment.Added.Name == name {
			return attachment
		}
	}
	require.Failf(t, "no attachment", "%s was not attached as an added symbol", name)
	return Attachment{}
}

// citations spells each candidate the way §4.3.3 cites it, `path:line`, so an
// ordering assertion reads as the list a reader would see.
func citations(decls []symbol.Decl) []string {
	out := make([]string, 0, len(decls))
	for _, d := range decls {
		out = append(out, d.Path+":"+strconv.Itoa(d.Line))
	}
	return out
}

// keysOf lists a decoded object's keys.
func keysOf(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	return keys
}
