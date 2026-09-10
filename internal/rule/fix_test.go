package rule

import (
	"testing"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// twoHunkDiff changes one file in two places, so a range can be inside the
// diff at both ends and inside no single hunk — which is the only thing §8.2.2
// says that §8.2.1 does not.
const twoHunkDiff = `--- a/app/Models/Order.php
+++ b/app/Models/Order.php
@@ -10,3 +10,3 @@
 $unchanged = DB::raw('untouched');
-$removed = DB::raw('gone');
+$added = DB::raw('new');
 $tail = 1;
@@ -40,1 +40,2 @@
 $keep = 1;
+$more = DB::raw('two');
`

// fixBlock is a well-formed §2.6.2 fix block, with overrides applied so a case
// can state exactly the one field it is about. The default pair rewrites the
// call the detector matches and keeps its argument, which is what makes the
// capture group's expansion observable rather than assumed.
func fixBlock(overrides map[string]any) map[string]any {
	block := map[string]any{"replace": `DB::raw\('([^']*)'\)`, "with": `DB::selectRaw('$1')`}
	for key, value := range overrides {
		block[key] = value
	}
	return block
}

// detecting builds the rule document a fix case needs: §2.6.1's detector, so
// there is a matched line at all, and the fix block under test.
func detecting(fix map[string]any) map[string]any {
	doc := regexDetect(nil)
	if fix != nil {
		doc["fix"] = fix
	}
	return doc
}

// fixMatcher compiles the single-rule corpus one case describes, which is the
// path Compile takes in a run rather than a Matcher assembled by hand.
func fixMatcher(t *testing.T, fix map[string]any) Matcher {
	t.Helper()
	matchers, err := Compile(resolveOne(t, detecting(fix)))
	require.NoError(t, err)
	require.Len(t, matchers, 1)
	return matchers[0]
}

// oneHit evaluates mixedDiff and returns the single hit it produces, which is
// the matched line every case below applies a fix to.
func oneHit(t *testing.T, matcher *Matcher) Hit {
	t.Helper()
	hits := Evaluate([]Matcher{*matcher}, hunksOf(t, mixedDiff))
	require.Len(t, hits, 1)
	return hits[0]
}

// aRuleRecord is the record a confirmed hit is written into, anchored on the
// line the fixture's hunk adds.
func aRuleRecord() finding.Finding {
	return finding.Finding{
		ID: "f1", Kind: finding.KindFinding, Role: "conventions", Class: "raw-sql",
		Rule: "no-raw-sql", Severity: finding.SeverityMedium, Unit: "u1",
		Anchor: finding.Anchor{
			Path: "app/Models/Order.php", Side: git.Right, StartLine: 11, Line: 11,
		},
		Summary:  "The added line builds SQL with DB::raw.",
		Evidence: "The rule's detector matched the added line.",
	}
}

// §2.6.2.1: `fix.replace` and `fix.with` are a regular expression and its
// replacement, applied to the matched line to produce a suggestion.
//
// The replacement expands `$1` rather than pasting `fix.with` whole, which is
// what lets a fix rewrite a line instead of only overwriting it. A fix that
// could not carry the matched text across would be usable on the one line its
// author had in front of them and wrong on the next.
func TestAFixRewritesTheMatchedLineIntoASuggestion(t *testing.T) {
	matcher := fixMatcher(t, fixBlock(nil))
	hit := oneHit(t, &matcher)

	text, produced := matcher.Replacement(&hit)

	require.True(t, produced)
	assert.Equal(t, `$added = DB::selectRaw('new');`, text)
	assert.Equal(t, `$added = DB::raw('new');`, hit.Text,
		"§2.6.2.3 produces text; the line the fix read is not rewritten in place")
}

// A fix that has nothing to say about the line says nothing. Each of these
// would otherwise reach the author as a suggestion asking them to accept the
// code they already wrote, and §1.6 spends the same trust on that comment as on
// a wrong one.
func TestAFixWithNothingToRewriteProducesNoSuggestion(t *testing.T) {
	for _, c := range []struct {
		name string
		fix  map[string]any
	}{
		{name: "no fix block at all", fix: nil},
		{name: "a replace that does not match the line",
			fix: fixBlock(map[string]any{"replace": `Cache::forget\(`})},
		{name: "a replacement identical to what it replaced",
			fix: fixBlock(map[string]any{"with": `DB::raw('$1')`})},
	} {
		t.Run(c.name, func(t *testing.T) {
			matcher := fixMatcher(t, c.fix)
			hit := oneHit(t, &matcher)

			text, produced := matcher.Replacement(&hit)

			assert.False(t, produced)
			assert.Empty(t, text)
		})
	}
}

// §2.6.2.2's ordinary case: a generated suggestion that passes §8.2's
// validation reaches the record before drafting.
func TestAValidatedSuggestionReachesTheRecord(t *testing.T) {
	matcher := fixMatcher(t, fixBlock(nil))
	hit := oneHit(t, &matcher)
	record := aRuleRecord()

	require.True(t, matcher.Suggest(&record, &hit, hunksOf(t, mixedDiff)))

	assert.Equal(t, `$added = DB::selectRaw('new');`, record.Suggestion)
}

// §2.6.2.2: a suggestion that fails validation is dropped while its record
// survives.
//
// The two are asserted apart, because they are two claims and only one of them
// is about the suggestion. Replacement is asked first and required to have
// produced something, so each case is a suggestion that existed and was
// dropped rather than one the fix never generated — and the record is compared
// whole against the copy taken before the call, so "survives" means every field
// of it and not merely that a pointer is still non-nil.
//
// A record can outlive its suggestion because the two say different things. The
// record says a rule's standard was broken at a place, which cr established
// from its own detection; the suggestion says what to write instead, which
// §2.6.2.4 says plainly cr cannot establish. Dropping the finding with the
// replacement would let an unusable fix silence a violation cr actually found.
func TestARecordOutlivesItsDroppedSuggestion(t *testing.T) {
	for _, c := range []struct {
		name   string
		anchor finding.Anchor
	}{
		{name: "§8.2.1: a line the diff does not contain",
			anchor: finding.Anchor{
				Path: "app/Models/Order.php", Side: git.Right, StartLine: 400, Line: 400,
			}},
		{name: "§8.2.1: a file the diff does not touch",
			anchor: finding.Anchor{
				Path: "app/Models/Invoice.php", Side: git.Right, StartLine: 11, Line: 11,
			}},
		{name: "§8.2.1: a range on the LEFT side",
			anchor: finding.Anchor{
				Path: "app/Models/Order.php", Side: git.Left, StartLine: 11, Line: 11,
			}},
		{name: "§8.2.2: a range spanning two hunks",
			anchor: finding.Anchor{
				Path: "app/Models/Order.php", Side: git.Right, StartLine: 11, Line: 41,
			}},
	} {
		t.Run(c.name, func(t *testing.T) {
			matcher := fixMatcher(t, fixBlock(nil))
			hit := oneHit(t, &matcher)
			record := aRuleRecord()
			record.Anchor = c.anchor
			before := record

			_, produced := matcher.Replacement(&hit)
			require.True(t, produced, "the fix must have generated something for a drop to mean anything")

			assert.False(t, matcher.Suggest(&record, &hit, hunksOf(t, twoHunkDiff)))
			assert.Empty(t, record.Suggestion, "§2.6.2.2 drops the suggestion")
			assert.Equal(t, before, record, "§2.6.2.2 keeps the record it belonged to")
		})
	}
}

// topOfFileDiff changes a file's first line, so its hunk's head range starts
// at line 1 — the lowest line §8.2.1's range can name.
const topOfFileDiff = `--- a/app/Models/Order.php
+++ b/app/Models/Order.php
@@ -1,2 +1,2 @@
-$removed = DB::raw('gone');
+$added = DB::raw('new');
 $tail = 1;
`

// §8.2.2's "within one hunk" counts the hunk's own first and last lines in,
// and §8.2.1's range may start on a file's first line. Every case above that
// places a suggestion sits strictly inside its hunk, so a validator that
// counted either edge out would pass them all — and would then drop, without
// a word, the fix for every finding anchored on an edge line.
func TestASuggestionMayReachEitherEdgeOfItsHunk(t *testing.T) {
	for _, c := range []struct {
		name   string
		diff   string
		anchor finding.Anchor
	}{
		{name: "a range starting on the hunk's first line", diff: mixedDiff,
			anchor: finding.Anchor{
				Path: "app/Models/Order.php", Side: git.Right, StartLine: 10, Line: 11,
			}},
		{name: "a range ending on the hunk's last line", diff: mixedDiff,
			anchor: finding.Anchor{
				Path: "app/Models/Order.php", Side: git.Right, StartLine: 11, Line: 12,
			}},
		{name: "a range on the file's first line", diff: topOfFileDiff,
			anchor: finding.Anchor{
				Path: "app/Models/Order.php", Side: git.Right, StartLine: 1, Line: 1,
			}},
	} {
		t.Run(c.name, func(t *testing.T) {
			matcher := fixMatcher(t, fixBlock(nil))
			hunks := hunksOf(t, c.diff)
			hits := Evaluate([]Matcher{matcher}, hunks)
			require.Len(t, hits, 1)
			record := aRuleRecord()
			record.Anchor = c.anchor

			require.True(t, matcher.Suggest(&record, &hits[0], hunks))

			assert.Equal(t, `$added = DB::selectRaw('new');`, record.Suggestion)
		})
	}
}

// A record already carrying the agent's own suggestion is left alone when the
// rule's fix produces nothing usable. §6.1's table lets an agent supply a
// suggestion of its own, and clearing the field on the failing path would
// delete that work in the name of a rule that generated nothing.
func TestADroppedRuleSuggestionDoesNotClearTheAgentsOwn(t *testing.T) {
	matcher := fixMatcher(t, fixBlock(nil))
	hit := oneHit(t, &matcher)
	record := aRuleRecord()
	record.Anchor.StartLine, record.Anchor.Line = 400, 400
	record.Suggestion = `$added = DB::table('orders')->select();`
	record.SuggestionOrigin = finding.OriginAgent

	assert.False(t, matcher.Suggest(&record, &hit, hunksOf(t, mixedDiff)))

	assert.Equal(t, `$added = DB::table('orders')->select();`, record.Suggestion)
	assert.Equal(t, finding.OriginAgent, record.SuggestionOrigin)
}

// §2.6 item 5: a rule file cr cannot use aborts the command, and internal/cli
// maps the error onto exit code 3. A `fix.replace` that is not a regular
// expression is such a file, exactly as an uncompilable `detect.pattern` is:
// its author wrote it to be run, and a corpus that quietly dropped it would
// review the change against every rule but that one and report full coverage.
func TestAFixPatternThatDoesNotCompileMakesTheRuleFileMalformed(t *testing.T) {
	for _, c := range []struct {
		name string
		doc  map[string]any
	}{
		{name: "beside a detect block", doc: detecting(fixBlock(map[string]any{"replace": `DB::raw(`}))},
		{name: "with no detect block", doc: map[string]any{"fix": fixBlock(map[string]any{"replace": `DB::raw(`})}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := Compile(resolveOne(t, c.doc))

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, "fix.replace", malformed.Field)
			assert.Contains(t, malformed.Error(), "no-raw-sql")
		})
	}
}
