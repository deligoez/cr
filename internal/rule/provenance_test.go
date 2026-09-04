package rule

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.6.2.4: a suggestion produced by `fix` carries `suggestion_origin: rule`.
//
// The field is the whole of what a reader downstream has to go on. §8.1.6 emits
// a cr-owned provenance region for a record carrying it, and finding's
// ruleAttributed reads it as the mark that a rule stands behind the record — so
// a suggestion that reached a record without it would be a machine-generated
// replacement the author is shown as if a person had written it.
func TestASuggestionProducedByAFixCarriesTheRuleOrigin(t *testing.T) {
	matcher := fixMatcher(t, fixBlock(nil))
	hit := oneHit(t, &matcher)
	record := aRuleRecord()

	require.True(t, matcher.Suggest(&record, &hit, hunksOf(t, mixedDiff)))

	assert.Equal(t, finding.OriginRule, record.SuggestionOrigin)
	assert.Equal(t, "no-raw-sql", record.Rule,
		"§2.6 item 3 names the rule the origin points at")
}

// The origin is minted from the matcher and never read back off the record.
//
// A record already claiming the agent's own origin is stamped `rule` when a
// fix's suggestion lands on it, because what the field describes is the text
// now in `suggestion` — and that text came from the rule. The alternative reads
// provenance from the same place the suggestion is being written to, which
// means the value can be anything a caller put there.
func TestTheRuleOriginIsMintedRatherThanCarriedOver(t *testing.T) {
	matcher := fixMatcher(t, fixBlock(nil))
	hit := oneHit(t, &matcher)
	record := aRuleRecord()
	record.Suggestion = `$added = DB::table('orders')->select();`
	record.SuggestionOrigin = finding.OriginAgent

	require.True(t, matcher.Suggest(&record, &hit, hunksOf(t, mixedDiff)))

	assert.Equal(t, `$added = DB::selectRaw('new');`, record.Suggestion)
	assert.Equal(t, finding.OriginRule, record.SuggestionOrigin,
		"the field describes the text now in `suggestion`, which the rule produced")
}

// No function of this package takes an origin, so no caller can supply one.
//
// This is §6.1.4's reasoning applied to the one origin cr writes outside
// citations. `suggestion_origin` is what §8.1.6 reads to disclose weak
// provenance to the author, so an agent able to pass it could hand cr `agent`
// for a replacement a rule generated and the disclosure would silently not
// happen. Keeping the value out of every signature is what makes that
// impossible rather than merely unlikely, and it is asserted from the source so
// a parameter added years from now fails here instead of opening the channel.
func TestNoSignatureInThisPackageAcceptsASuggestionOrigin(t *testing.T) {
	sources, err := os.ReadDir(".")
	require.NoError(t, err)

	accepting := make([]string, 0)
	for _, source := range sources {
		name := source.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
		require.NoError(t, parseErr)
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && namesAnOrigin(fn.Type.Params) {
				accepting = append(accepting, name+": "+fn.Name.Name)
			}
		}
	}

	assert.Empty(t, accepting, "§2.6.2.4's origin is stamped by cr and supplied by nobody")
}

// namesAnOrigin reports whether a parameter list mentions the Origin type at
// all, so a bare Origin, a pointer to one, and a slice of them all count.
func namesAnOrigin(params *ast.FieldList) bool {
	if params == nil {
		return false
	}
	found := false
	ast.Inspect(params, func(node ast.Node) bool {
		if named, ok := node.(*ast.Ident); ok && named.Name == "Origin" {
			found = true
		}
		return !found
	})
	return found
}

// §2.6.2.4: a machine-generated suggestion is dropped when the agent does not
// confirm it, and §2.6.1.5 says the same of the hit behind it.
//
// Confirmation is a record naming the rule. Each case below is a hit that a fix
// could rewrite and an agent that did not stand behind it, and none of them
// leaves the generated text anywhere a payload could read it: §8.1 renders a
// posted comment from a record, so text that reached no record reaches no
// author.
func TestAnUnconfirmedMachineSuggestionNeverReachesARecord(t *testing.T) {
	generated := `$added = DB::selectRaw('new');`
	for _, c := range []struct {
		name string
		rule string
	}{
		{name: "a record naming another rule", rule: "handle-every-error"},
		{name: "a record naming no rule at all", rule: ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			matcher := fixMatcher(t, fixBlock(nil))
			hit := oneHit(t, &matcher)
			record := aRuleRecord()
			record.Rule = c.rule

			text, produced := matcher.Replacement(&hit)
			require.True(t, produced, "the fix must have generated something for a drop to mean anything")
			require.Equal(t, generated, text)

			assert.False(t, matcher.Suggest(&record, &hit, hunksOf(t, mixedDiff)))
			assert.Empty(t, record.Suggestion)
			assert.Empty(t, record.SuggestionOrigin)
		})
	}
}

// A hit the agent wrote no record for at all leaves nothing behind, which is
// the case the two above cannot show: there is no record to inspect.
//
// So the whole round is the subject. Two rules hit the same line, the agent
// confirms one of them, and the records that exist afterwards are read for the
// other one's replacement. It appears in none of them, because the only
// function that puts suggestion text anywhere is handed a record, and no record
// was ever written for that hit.
func TestAHitNoRecordWasWrittenForLeavesNoSuggestionBehind(t *testing.T) {
	confirmed := fixMatcher(t, fixBlock(nil))
	unconfirmed := unconfirmedMatcher(t)

	hits := Evaluate([]Matcher{confirmed, unconfirmed}, hunksOf(t, mixedDiff))
	require.Len(t, hits, 2, "both rules hit the same added line")

	record := aRuleRecord()
	require.True(t, confirmed.Suggest(&record, &hits[0], hunksOf(t, mixedDiff)))
	records := []finding.Finding{record}

	dropped, produced := unconfirmed.Replacement(&hits[1])
	require.True(t, produced, "the unconfirmed rule's fix did generate text")
	for _, written := range records {
		assert.NotEqual(t, dropped, written.Suggestion,
			"a hit the agent did not confirm reaches no record, and so no payload")
	}
	assert.Equal(t, `$added = DB::selectRaw('new');`, records[0].Suggestion)
}

// unconfirmedMatcher is a second rule matching the same line as the first, with
// a fix of its own that produces distinguishable text.
func unconfirmedMatcher(t *testing.T) Matcher {
	t.Helper()
	doc := regexDetect(nil)
	doc["fix"] = map[string]any{"replace": `DB::raw\(`, "with": `DB::statement(`}
	corpus, err := Resolve(t.TempDir(), t.TempDir(), profileFile,
		embedded(t, ruleDoc("handle-every-error", doc)))
	require.NoError(t, err)
	matchers, err := Compile(corpus)
	require.NoError(t, err)
	require.Len(t, matchers, 1)
	return matchers[0]
}

// The anchor a machine suggestion would be posted against is the record's own,
// which §9.2 numbers on the head. It is asserted here because §8.1.6's
// provenance region and §8.2's validation both read it, and a stamp that moved
// it would disclose the right origin against the wrong lines.
func TestStampingTheOriginMovesNothingElseOnTheRecord(t *testing.T) {
	matcher := fixMatcher(t, fixBlock(nil))
	hit := oneHit(t, &matcher)
	record := aRuleRecord()
	before := record

	require.True(t, matcher.Suggest(&record, &hit, hunksOf(t, mixedDiff)))

	before.Suggestion, before.SuggestionOrigin = record.Suggestion, record.SuggestionOrigin
	assert.Equal(t, before, record)
	assert.Equal(t, git.Right, record.Anchor.Side)
}
