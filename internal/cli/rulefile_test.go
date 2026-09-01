package cli

import (
	"fmt"
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.6.5 makes a malformed rule file abort with exit code 3, and the abort must
// survive the wrapping a command adds on the way out.
//
// The three cases below are the three shapes §2.6.5 covers with one sentence: a
// row the table requires and the file omits, a key the table does not have, and
// an `axis` outside §1.5's closed set. The last leaves this package through
// axis.InvalidError rather than rule.MalformedError, which already maps to the
// same code — so the assertion here is that a rule file reaches it, not that
// the mapping exists.
func TestAMalformedRuleFileExitsWithTheFileCode(t *testing.T) {
	for _, c := range []struct{ name, content string }{
		{
			name:    "a required row is missing",
			content: `{"id": "handle-every-error", "title": "Handle every error."}`,
		},
		{
			name:    "a key §2.6's table does not have",
			content: `{"id": "handle-every-error", "instructions": "Judge the error handling."}`,
		},
		{
			name:    "not JSON at all",
			content: "id: handle-every-error\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := rule.Parse("rules/handle-every-error.json", []byte(c.content))
			require.Error(t, err)

			var malformed *rule.MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, ExitFile, exitCodeFor(err))
			assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("loading rules: %w", err)))
		})
	}

	t.Run("an axis outside §1.5", func(t *testing.T) {
		_, err := rule.Parse("rules/handle-every-error.json", []byte(
			`{"id": "handle-every-error", "title": "Handle every error.",`+
				` "rationale": "A dropped error is a wrong answer nobody sees.",`+
				` "class": "unchecked-error", "axis": "security"}`))
		require.Error(t, err)

		var invalid *axis.InvalidError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, ExitFile, exitCodeFor(err))
		assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("loading rules: %w", err)))
	})
}
