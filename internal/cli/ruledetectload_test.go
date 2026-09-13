package cli

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
)

// malformedDetectRule is one rule whose file is well formed in every row of
// §2.6's table and unusable only in its `detect` block.
type malformedDetectRule struct {
	// name is the subtest name; it reaches no shell, but carries no shell
	// metacharacter either.
	name string
	// mode and pattern are the detect block.
	mode, pattern string
	// field is the field the refusal names.
	field string
	// problem completes the refusal's sentence for rule id and the pattern.
	problem func(id, pattern string) string
}

// malformedDetectRules are audit round 7 row list-8-5's two probes: a mode
// §2.6.1.2 does not admit, and a pattern that is not a Go regexp.
func malformedDetectRules() []malformedDetectRule {
	return []malformedDetectRule{
		{
			name: "detect.mode glob", mode: "glob", pattern: `DB::raw`, field: "detect.mode",
			problem: func(id, _ string) string {
				return fmt.Sprintf("of rule %q is %q, and §2.6.1.2 admits only %q in v0.1", id, "glob", rule.ModeRegex)
			},
		},
		{
			name: "detect.pattern an open group", mode: rule.ModeRegex, pattern: "(", field: "detect.pattern",
			problem: func(id, pattern string) string {
				_, err := regexp.Compile(pattern)
				return fmt.Sprintf("of rule %q is not a Go regexp: %v", id, err)
			},
		},
	}
}

// writeMalformedDetectRule writes id as a global rule of class, carrying c's
// detect block, and returns the file it wrote.
func writeMalformedDetectRule(t *testing.T, layout state.Layout, id, class string, c malformedDetectRule) string {
	t.Helper()
	body := `{"id":"` + id + `","title":"The ` + id + ` standard.","rationale":"Why ` + id + ` exists.",` +
		`"class":"` + class + `","detect":{"mode":"` + c.mode + `","pattern":"` + c.pattern + `"}}`
	path := layout.Rule(id)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// assertMalformedDetectRefusal holds err to §2.6 item 5's abort: exit code 3,
// the rule-file hint, and a refusal naming the file, the field and the rule.
func assertMalformedDetectRefusal(t *testing.T, err error, path, id string, c malformedDetectRule) {
	t.Helper()
	var malformed *rule.MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, &rule.MalformedError{File: path, Field: c.field, Problem: c.problem(id, c.pattern)}, malformed)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, "repair the rule file the message names: the key it names, or the file "+
		"itself when it cannot be read; §2.6's table is the whole of what a rule may carry", hintFor(err))
}

// §2.6 item 5 through `cr rules list`: a rule whose detect block cr cannot run
// is malformed wherever the corpus is loaded, so listing it aborts with exit
// code 3 naming its file, as `cr rules check` does — rather than printing it as
// an effective rule, which is what audit round 7 row list-8-5 measured.
func TestRulesListRefusesARuleWhoseDetectBlockCannotRun(t *testing.T) {
	for _, c := range malformedDetectRules() {
		t.Run(c.name, func(t *testing.T) {
			layout := layeredHome(t)
			path := writeMalformedDetectRule(t, layout, "no-raw-sql", "sql-injection", c)

			printed, err := runCLIPrinting(t, "rules", "list", "--repo", harvestSlug)

			assertMalformedDetectRefusal(t, err, path, "no-raw-sql", c)
			assert.Empty(t, printed, "a refused corpus lists no rule as effective")
		})
	}
}

// §2.6 item 5 through `cr record`: a record naming a rule makes the command
// load the round's corpus for §2.6's class row, and a rule whose detect block
// cr cannot run aborts it with exit code 3 naming the file, storing nothing.
//
// The record's class differs from the rule's on purpose. The class row is read
// off the loaded corpus before anything compiles it, so a loader that let the
// block through would hold the record to a rule it cannot use and refuse the
// line with exit code 1; a matching class would reach §2.6.2.1's fix
// generation, which compiles, and hide where the check lives.
func TestRecordRefusesARuleWhoseDetectBlockCannotRun(t *testing.T) {
	for _, c := range malformedDetectRules() {
		t.Run(c.name, func(t *testing.T) {
			layout := recordedHome(t)
			path := writeMalformedDetectRule(t, layout, ruleID, ruleClass, c)
			named := aRecord("f1", "u1")
			named["rule"] = ruleID
			named["class"] = "dropped-error"
			file := writeRecordFile(t, "merged.ndjson", named)

			_, err := runRecord(t, recordPR, file, "--repo", recordSlug)

			assertMalformedDetectRefusal(t, err, path, ruleID, c)
			assert.Empty(t, recordStore(t, layout), "a refused corpus records nothing")
		})
	}
}
