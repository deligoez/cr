package rule

import (
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// detector is the `detect` block that puts a rule on §2.6.1.1's mechanical
// side. Its content does not matter here — presence is what §2.6.1 splits on.
var detector = map[string]any{"mode": ModeRegex, "pattern": `_ = err`}

// injectionCorpus resolves a corpus holding one rule of each of the four kinds
// the split can produce: detect-less on the axis being asked for, detect-less
// on another axis, and a detecting rule on each of the two.
func injectionCorpus(t *testing.T) []Resolved {
	t.Helper()
	corpus, err := Resolve(rulesDir(t,
		ruleDoc("handle-every-error", map[string]any{"axis": axis.Convention}),
		ruleDoc("name-the-boundary", map[string]any{"axis": axis.Correctness}),
		ruleDoc("no-blank-error", map[string]any{
			"axis": axis.Convention, "detect": detector,
		}),
		ruleDoc("no-silent-retry", map[string]any{
			"axis": axis.Correctness, "detect": detector,
		}),
	), t.TempDir(), profileFile, nil)
	require.NoError(t, err)
	require.Len(t, corpus, 4)
	return corpus
}

// §2.6.1.4: a rule without a `detect` block is injected into its axis role's
// prompt as text.
//
// The two selectors are asserted together because either alone would pass a
// wrong implementation. Selecting on the axis and not on the block would inject
// a mechanically detected rule as prose as well, so the role would be asked to
// look for what cr already found; selecting on the block and not on the axis
// would put a convention standard into the correctness role's prompt, where it
// is an instruction nobody there can act on.
func TestOnlyTheDetectLessRulesOfAnAxisAreInjected(t *testing.T) {
	corpus := injectionCorpus(t)

	assert.Equal(t, []string{"handle-every-error"}, ids(Injected(corpus, axis.Convention)))
	assert.Equal(t, []string{"name-the-boundary"}, ids(Injected(corpus, axis.Correctness)))
	assert.Empty(t, Injected(corpus, axis.Test),
		"an axis no rule enforces is injected nothing rather than everything")
}

// §2.6.1's split is exhaustive and disjoint: every resolved rule is either
// compiled into a matcher or injected as text, and none is both.
//
// This is the property that makes "a rule cr does not enforce" impossible to
// arrive at by accident. A rule that fell into neither half would be written,
// validated, resolved, and then silently enforced by nothing at all — and the
// coverage report would say the convention axis ran.
func TestEveryResolvedRuleIsEitherCompiledOrInjected(t *testing.T) {
	corpus := injectionCorpus(t)

	matchers, err := Compile(corpus)
	require.NoError(t, err)
	compiled := make([]string, 0, len(matchers))
	for at := range matchers {
		compiled = append(compiled, matchers[at].Rule.ID)
	}

	injected := make([]string, 0, len(corpus))
	for _, id := range []string{axis.Intent, axis.Correctness, axis.Convention, axis.Test} {
		injected = append(injected, ids(Injected(corpus, id))...)
	}

	assert.ElementsMatch(t, ids(corpus), append(compiled, injected...),
		"§2.6.1 splits the corpus in two, and the two halves are the whole of it")
	for _, id := range compiled {
		assert.NotContains(t, injected, id, "%s is enforced twice over", id)
	}
}

// The injected text carries the standard and why it exists, so the role reads
// what §2.6 item 4 has a record quote to the author.
//
// The id is asserted alongside them because §2.6 item 3 requires every record
// produced by this rule to carry it, and the prompt is the only place the agent
// can learn it. The resolved path is asserted because §2.6 item 2 lets a higher
// layer override a lower one whole: a role reading a standard it disagrees with
// has to be told which of the three files it is written in.
func TestTheInjectedTextCarriesTheTitleAndTheRationale(t *testing.T) {
	corpus := injectionCorpus(t)
	injected := Injected(corpus, axis.Convention)
	require.Len(t, injected, 1)

	text := injected[0].Injection()
	rule := injected[0].Rule
	require.NotEmpty(t, rule.Title)
	require.NotEmpty(t, rule.Rationale)

	for _, part := range []string{
		rule.ID, rule.Title, rule.Rationale, injected[0].Path,
		rule.Axis, rule.Class, string(rule.Severity), string(rule.Kind),
	} {
		assert.Containsf(t, text, part, "the injected text drops %q", part)
	}
}

