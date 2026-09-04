package rule

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/deligoez/cr/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mixedDiff carries all three kinds of line one hunk can hold, each of them
// matching the same pattern: one unchanged context line, one removed line, and
// one added line. §2.6.1.1 admits exactly the third.
const mixedDiff = `--- a/app/Models/Order.php
+++ b/app/Models/Order.php
@@ -10,3 +10,3 @@
 $unchanged = DB::raw('untouched');
-$removed = DB::raw('gone');
+$added = DB::raw('new');
 $tail = 1;
`

// deletionDiff removes a matching line and adds none, which is the hunk
// internal/git numbers on §9.2's LEFT.
const deletionDiff = `--- a/app/Legacy/Report.php
+++ b/app/Legacy/Report.php
@@ -5,2 +5,1 @@
-$gone = DB::raw('deleted');
 $kept = 1;
`

// hunksOf parses a unified diff the way §3.4 ingests one.
func hunksOf(t *testing.T, patch string) []git.Hunk {
	t.Helper()
	hunks, err := git.ParseHunks(patch)
	require.NoError(t, err)
	return hunks
}

// matcherFor pairs a rule with its compiled pattern, which is what §2.6.1.2
// hands detection.
func matcherFor(t *testing.T, id, pattern string, overrides map[string]any) Matcher {
	t.Helper()
	r, err := Load(rulePath(t, id, overrides))
	require.NoError(t, err)
	return Matcher{Rule: r, Pattern: regexp.MustCompile(pattern)}
}

// rulePath writes one rule file and returns its path.
func rulePath(t *testing.T, id string, overrides map[string]any) string {
	t.Helper()
	return filepath.Join(rulesDir(t, ruleDoc(id, overrides)), id+fileExt)
}

// §2.6.1.1: a rule's detect block is evaluated over the added and modified
// RIGHT-side lines of the diff only. All three lines of the hunk match the
// pattern, so what the assertion measures is the filter and not the regexp.
//
// The removed line is the expensive one to get wrong. A comment on a line the
// change deleted asks its author to fix code that is no longer there, and §1.6
// says that trust is spent once.
func TestDetectEvaluatesOnlyTheAddedAndModifiedRightSideLines(t *testing.T) {
	hits := Evaluate(
		[]Matcher{matcherFor(t, "no-raw-sql", `DB::raw`, nil)},
		hunksOf(t, mixedDiff),
	)

	require.Len(t, hits, 1, "the context line and the removed line both matched the pattern")
	assert.Equal(t, Hit{
		RuleID: "no-raw-sql",
		Path:   "app/Models/Order.php",
		Line:   11,
		Text:   "$added = DB::raw('new');",
	}, hits[0])
}

// A hunk that adds no line is numbered on LEFT, and every line it carries is
// one the change removed. A rule states how code must be written, and a line
// the change deleted is not written anywhere any more.
func TestARemovedLineProducesNoHit(t *testing.T) {
	hunks := hunksOf(t, deletionDiff)
	require.Len(t, hunks, 1)
	require.Equal(t, git.Left, hunks[0].Side, "the fixture must be a deletion for this to prove anything")
	require.NotEmpty(t, hunks[0].Changed)

	hits := Evaluate([]Matcher{matcherFor(t, "no-raw-sql", `DB::raw`, nil)}, hunks)

	assert.Empty(t, hits)
}

// "Never over the whole repository" is in the signature rather than in a check.
// Evaluate is handed hunks and nothing else — no repository root, no head, no
// path to open — so an unchanged line matching the pattern cannot reach it.
//
// The import list is the structural half. A file that names no package able to
// read a file cannot read one, whatever a later edit believes it is doing, and
// this fails the moment one is added rather than the round a comment lands on
// untouched code.
func TestDetectionCanReadNothingButTheDiff(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "detect.go", nil, parser.SkipObjectResolution)
	require.NoError(t, err)

	imported := make([]string, 0, len(parsed.Imports))
	for _, spec := range parsed.Imports {
		path, unquoteErr := strconv.Unquote(spec.Path.Value)
		require.NoError(t, unquoteErr)
		imported = append(imported, path)
	}
	slices.Sort(imported)

	assert.Equal(t, []string{
		"github.com/deligoez/cr/internal/git",
		"github.com/deligoez/cr/internal/glob",
		"regexp",
	}, imported, "§2.6.1.1 keeps detection off the repository, so it imports nothing that could read it")
}

// §2.6's `globs` narrows the paths a rule applies to, and an empty list means
// all source. The two readings are opposite ways round from §2.4's
// `tests.globs`, where an empty list recognises nothing, so each is asserted
// rather than inferred from the other.
func TestGlobsNarrowThePathsARuleAppliesTo(t *testing.T) {
	for _, c := range []struct {
		name    string
		globs   []string
		matched bool
	}{
		{name: "no globs means all source", globs: nil, matched: true},
		{name: "a glob covering the path", globs: []string{"app/**"}, matched: true},
		{name: "a glob covering another tree", globs: []string{"database/**"}, matched: false},
		{name: "one of several globs is enough", globs: []string{"database/**", "app/**"}, matched: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			overrides := map[string]any{}
			if c.globs != nil {
				overrides["globs"] = c.globs
			}
			hits := Evaluate(
				[]Matcher{matcherFor(t, "no-raw-sql", `DB::raw`, overrides)},
				hunksOf(t, mixedDiff),
			)

			assert.Len(t, hits, map[bool]int{true: 1, false: 0}[c.matched])
		})
	}
}

// §2.6's `exempt` excludes paths, for legacy areas. It is asked before `globs`
// and not after, because that is what excluding means: a directory named by
// both rows stays out. The other order would make `exempt` a suggestion — the
// legacy tree would be enforced against, which is the one arrangement its
// author wrote the row to prevent.
func TestExemptExcludesAPathThatGlobsSelected(t *testing.T) {
	legacy := `--- a/app/Legacy/Report.php
+++ b/app/Legacy/Report.php
@@ -5,1 +5,2 @@
 $kept = 1;
+$added = DB::raw('new');
`
	rule := matcherFor(t, "no-raw-sql", `DB::raw`, map[string]any{
		"globs":  []string{"app/**"},
		"exempt": []string{"app/Legacy/**"},
	})

	assert.Empty(t, Evaluate([]Matcher{rule}, hunksOf(t, legacy)),
		"app/Legacy sits inside app/**, and exempt subtracts from globs")
	assert.Len(t, Evaluate([]Matcher{rule}, hunksOf(t, mixedDiff)), 1,
		"the rest of app/** is still enforced, so the exemption is a hole and not a switch")
}

// Applies is asked once per file rather than once per line, so it is worth
// pinning on its own: the rows it reads decide whether a whole file is looked
// at, and a wrong answer here is silent in both directions.
func TestAppliesReadsGlobsAndExemptDirectly(t *testing.T) {
	for _, c := range []struct {
		name          string
		globs, exempt []string
		path          string
		applies       bool
	}{
		{name: "empty globs means all source", path: "database/seed.php", applies: true},
		{name: "exempt with no globs still excludes", exempt: []string{"vendor/**"},
			path: "vendor/lib/a.php", applies: false},
		{name: "exempt wins over a matching glob", globs: []string{"app/**"},
			exempt: []string{"app/Legacy/**"}, path: "app/Legacy/Report.php", applies: false},
		{name: "a glob at the tree root", globs: []string{"*.php"}, path: "index.php", applies: true},
		{name: "a single star does not cross a separator", globs: []string{"app/*.php"},
			path: "app/Models/Order.php", applies: false},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := Rule{Globs: c.globs, Exempt: c.exempt}

			assert.Equal(t, c.applies, r.Applies(c.path))
		})
	}
}

// Hits come back in corpus order and then in diff order. §2.6.1.6 appends every
// one of them to `rule-stats.ndjson`, which §6.2.5 later matches a citation
// against positionally, so the sequence is part of what is written rather than
// a convenience of the loop.
func TestHitsComeBackInCorpusOrderThenDiffOrder(t *testing.T) {
	second := `--- a/app/Models/Invoice.php
+++ b/app/Models/Invoice.php
@@ -1,1 +1,3 @@
 <?php
+$a = DB::raw('one');
+$b = DB::raw('two');
`
	hits := Evaluate(
		[]Matcher{
			matcherFor(t, "no-raw-sql", `DB::raw`, nil),
			matcherFor(t, "handle-every-error", `DB::`, nil),
		},
		append(hunksOf(t, mixedDiff), hunksOf(t, second)...),
	)

	require.Len(t, hits, 6)
	assert.Equal(t, []string{
		"no-raw-sql", "no-raw-sql", "no-raw-sql",
		"handle-every-error", "handle-every-error", "handle-every-error",
	}, ruleIDs(hits))
	assert.Equal(t, []int{11, 2, 3, 11, 2, 3}, lines(hits))
}

// ruleIDs reads the hit list as the rules that produced it.
func ruleIDs(hits []Hit) []string {
	written := make([]string, 0, len(hits))
	for _, h := range hits {
		written = append(written, h.RuleID)
	}
	return written
}

// lines reads the hit list as the head-side lines it points at.
func lines(hits []Hit) []int {
	numbers := make([]int, 0, len(hits))
	for _, h := range hits {
		numbers = append(numbers, h.Line)
	}
	return numbers
}
