package cli

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/deligoez/cr/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.6.1.2 makes an unusable `detect` block abort with exit code 3, and the
// abort must survive the wrapping a command adds on the way out.
//
// The two cases are the two the section names: a pattern that is not a Go
// regexp, and a `mode` outside the one value v0.1 admits. Both leave
// internal/rule as a rule.MalformedError, which is the same type §2.6.5's
// malformed file uses — so what is asserted here is that a fault found while
// compiling a rule reaches the file code too, not that the mapping exists.
func TestAnUnusableDetectBlockExitsWithTheFileCode(t *testing.T) {
	for _, c := range []struct{ name, pattern, mode string }{
		{name: "a pattern that is not a Go regexp", pattern: "DB::raw(unclosed", mode: "regex"},
		{name: "a mode v0.1 does not admit", pattern: `DB::raw\(`, mode: "glob"},
		{name: "a mode left blank", pattern: `DB::raw\(`, mode: ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			corpus := corpusWithDetect(t, c.pattern, c.mode)

			_, err := rule.Compile(corpus)
			require.Error(t, err)

			var malformed *rule.MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, ExitFile, exitCodeFor(err))
			assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("compiling rules: %w", err)))
		})
	}
}

// corpusWithDetect resolves a one-rule corpus whose rule carries the given
// detect block, through the same path a profile's `rules` array takes.
func corpusWithDetect(t *testing.T, pattern, mode string) []rule.Resolved {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"id":        "no-raw-sql",
		"title":     "Query the builder, never DB::raw.",
		"rationale": "A raw fragment skips parameter binding, so an injection reads as ordinary code.",
		"class":     "sql-injection",
		"detect":    map[string]any{"pattern": pattern, "mode": mode},
	})
	require.NoError(t, err)

	corpus, err := rule.Resolve(t.TempDir(), t.TempDir(), "profiles/laravel-pest.json",
		[]json.RawMessage{body})
	require.NoError(t, err, "the rule file itself is well formed; only the detect block is not")
	return corpus
}
