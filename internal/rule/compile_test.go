package rule

import (
	"maps"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// regexDetect is a well-formed §2.6.1 detect block, with overrides applied so a
// case can state exactly the one field it is about.
func regexDetect(overrides map[string]any) map[string]any {
	block := map[string]any{"pattern": `DB::raw\(`, "mode": ModeRegex}
	maps.Copy(block, overrides)
	return map[string]any{"detect": block}
}

// resolveOne resolves a corpus holding a single rule embedded in a profile,
// under the id every case below shares.
func resolveOne(t *testing.T, overrides map[string]any) []Resolved {
	t.Helper()
	corpus, err := Resolve(t.TempDir(), t.TempDir(), profileFile,
		embedded(t, ruleDoc("no-raw-sql", overrides)))
	require.NoError(t, err)
	return corpus
}

// §2.6.1.2 is what Compile is for, and the pattern it produces has to be the
// one the rule wrote. A matcher built from a pattern nobody wrote would report
// hits under that rule's id, quoting that rule's rationale, about a standard it
// never stated.
func TestCompileProducesTheRulesOwnPattern(t *testing.T) {
	matchers, err := Compile(resolveOne(t, regexDetect(nil)))
	require.NoError(t, err)

	require.Len(t, matchers, 1)
	assert.Equal(t, "no-raw-sql", matchers[0].Rule.ID)
	assert.True(t, matchers[0].Pattern.MatchString("$x = DB::raw('a');"))
	assert.False(t, matchers[0].Pattern.MatchString("$x = DB::table('a');"))
}

// A rule with no detect block produces no matcher. It is not dropped from the
// review — §2.6.1.4 injects it into its axis role's prompt as text — but that
// is a different mechanism, and the guarantee this one makes is that every
// matcher it hands back holds a pattern that compiled.
func TestARuleWithoutADetectBlockProducesNoMatcher(t *testing.T) {
	corpus, err := Resolve(t.TempDir(), t.TempDir(), profileFile, embedded(t,
		ruleDoc("handle-every-error", nil),
		ruleDoc("no-raw-sql", regexDetect(nil)),
		ruleDoc("prefer-value-objects", nil),
	))
	require.NoError(t, err)

	matchers, err := Compile(corpus)
	require.NoError(t, err)

	require.Len(t, matchers, 1)
	assert.Equal(t, "no-raw-sql", matchers[0].Rule.ID)
}

// §2.6.1.2: a pattern that fails to compile aborts, naming the rule. It is not
// a rule cr may quietly leave out — its author wrote it to enforce a standard,
// and a corpus that dropped it would review the change against every rule but
// that one and then report full coverage.
func TestAPatternThatDoesNotCompileAbortsNamingTheRule(t *testing.T) {
	_, err := Compile(resolveOne(t, regexDetect(map[string]any{
		"pattern": `DB::raw(unclosed`,
	})))

	var malformed *MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "detect.pattern", malformed.Field)
	assert.Contains(t, malformed.Problem, `"no-raw-sql"`, "§2.6.1.2 names the rule")
	assert.Contains(t, err.Error(), profileFile, "and the file it is written in")
}

// §2.6.1.2 closes `detect.mode` at `regex` in v0.1.
//
// The blank case is the one worth deciding out loud. §2.6's table gives `axis`,
// `severity` and `kind` a default in the table itself and gives `mode` none, so
// a block that leaves it blank has not said how its pattern applies. Reading
// `regex` into it would be cr choosing a semantics for a detector its author
// never finished describing — right today, and wrong the first time a second
// mode exists, against a file written before there was a second mode to mean.
//
// The value is matched exactly. A `REGEX` accepted case-insensitively would
// make the closed set larger than the one word §2.6.1.2 wrote.
func TestAModeOtherThanRegexAborts(t *testing.T) {
	for _, mode := range []string{"glob", "literal", "substring", "REGEX", "Regex", " regex", ""} {
		t.Run("mode "+mode, func(t *testing.T) {
			_, err := Compile(resolveOne(t,
				regexDetect(map[string]any{"mode": mode})))

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, "detect.mode", malformed.Field)
			assert.Contains(t, malformed.Problem, `"no-raw-sql"`)
			assert.Contains(t, malformed.Problem, ModeRegex,
				"the refusal names the one value it would have accepted")
		})
	}
}

// The mode is read before the pattern. A block declaring a mode cr does not
// have is a block whose pattern cr has no way to read, so compiling it first
// would answer a question about a syntax its author was not writing in — and a
// pattern valid in one dialect and not in Go's would be reported as a broken
// regular expression rather than as an unsupported mode.
func TestAnUnsupportedModeIsReportedBeforeThePatternIsRead(t *testing.T) {
	_, err := Compile(resolveOne(t, regexDetect(map[string]any{
		"mode":    "glob",
		"pattern": "app/**/[Order",
	})))

	var malformed *MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "detect.mode", malformed.Field)
}

// §2.6.1.2's abort names the file, and the file differs by layer: a rule file
// is its own, and a rule embedded in a profile is the profile's. Naming the
// wrong one sends its author to a file that does not carry the block.
func TestTheAbortNamesTheFileTheRuleIsWrittenIn(t *testing.T) {
	t.Run("a rule file", func(t *testing.T) {
		dir := rulesDir(t, ruleDoc("no-raw-sql",
			regexDetect(map[string]any{"mode": "glob"})))
		corpus, err := Resolve(dir, t.TempDir(), profileFile, nil)
		require.NoError(t, err)

		_, err = Compile(corpus)

		var malformed *MalformedError
		require.ErrorAs(t, err, &malformed)
		assert.Equal(t, filepath.Join(dir, "no-raw-sql"+fileExt), malformed.File)
	})

	t.Run("a rule embedded in a profile", func(t *testing.T) {
		_, err := Compile(resolveOne(t,
			regexDetect(map[string]any{"mode": "glob"})))

		var malformed *MalformedError
		require.ErrorAs(t, err, &malformed)
		assert.Equal(t, profileFile, malformed.File)
	})
}

// Resolve records the file each rule was read out of, which is what makes the
// abort above able to name one. Neither the layer nor the id is a file: two
// layers can spell one id, and a profile's array has no file name of its own.
func TestResolveRecordsTheFileEachRuleWasReadFrom(t *testing.T) {
	repo := rulesDir(t, ruleDoc("from-repo", nil))
	global := rulesDir(t, ruleDoc("from-global", nil))

	corpus, err := Resolve(repo, global, profileFile, embedded(t, ruleDoc("from-profile", nil)))
	require.NoError(t, err)

	require.Len(t, corpus, 3)
	assert.Equal(t, filepath.Join(repo, "from-repo"+fileExt), corpus[0].Path)
	assert.Equal(t, filepath.Join(global, "from-global"+fileExt), corpus[1].Path)
	assert.Equal(t, profileFile, corpus[2].Path)
}
