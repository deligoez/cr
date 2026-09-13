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
// internal/rule as a rule.MalformedError from the loader, which is the same
// type §2.6.5's malformed file uses — so what is asserted here is that a fault
// found in a detect block reaches the file code too, not that the mapping
// exists.
func TestAnUnusableDetectBlockExitsWithTheFileCode(t *testing.T) {
	for _, c := range []struct{ name, pattern, mode string }{
		{name: "a pattern that is not a Go regexp", pattern: "DB::raw(unclosed", mode: "regex"},
		{name: "a mode v0.1 does not admit", pattern: `DB::raw\(`, mode: "glob"},
		{name: "a mode left blank", pattern: `DB::raw\(`, mode: ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := resolveWithDetect(t, c.pattern, c.mode)
			require.Error(t, err)

			var malformed *rule.MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, ExitFile, exitCodeFor(err))
			assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("compiling rules: %w", err)))
		})
	}
}

// resolveWithDetect resolves a one-rule corpus whose rule carries the given
// detect block, through the same path a profile's `rules` array takes, and
// returns what the loader said about it.
func resolveWithDetect(t *testing.T, pattern, mode string) error {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"id":        "no-raw-sql",
		"title":     "Query the builder, never DB::raw.",
		"rationale": "A raw fragment skips parameter binding, so an injection reads as ordinary code.",
		"class":     "sql-injection",
		"detect":    map[string]any{"pattern": pattern, "mode": mode},
	})
	require.NoError(t, err)

	_, err = rule.Resolve(t.TempDir(), t.TempDir(), "profiles/laravel-pest.json",
		[]json.RawMessage{body})
	return err
}
