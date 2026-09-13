package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §6.1.4 through `cr record` under another letter case. encoding/json binds
// `"Thread_ID"` to the thread id exactly as it binds `"thread_id"`, and the
// thread id is a field cr stores as it arrives, so a fence that read keys by
// their exact spelling would let a record claim a GitHub thread before any
// post. The line is refused with exit code 1 naming §6.1's spelling of the
// field, and nothing is stored.
func TestRecordRefusesAComputedFieldUnderAnyKeyCase(t *testing.T) {
	for key, tc := range map[string]struct{ field, value string }{
		"Thread_ID": {field: "thread_id", value: "PRRT_kwDOAbCd"},
		"GRADE":     {field: "grade", value: "cited"},
	} {
		t.Run(key, func(t *testing.T) {
			layout := detectedHome(t)
			runRulesCheck(t)
			supplied := confirming("f1", 4)
			supplied[key] = tc.value

			_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", supplied),
				"--repo", fixtureSlug)

			var rejected *state.ReservedFieldError
			require.ErrorAs(t, err, &rejected, "§6.1.4's refusal of a computed field")
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Equal(t, tc.field, rejected.Field)
			assert.Empty(t, storedFindings(t, layout))
		})
	}
}
