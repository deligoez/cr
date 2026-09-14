package cli

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/rule"
)

// §2.6 item 5 and §2.6.1.2 through `cr rules list`: a detect block whose
// pattern is absent or empty is malformed, because an empty regular expression
// matches every changed RIGHT-side line and would turn the whole diff into hits
// of that rule. Audit round 14 row list-9-2 measured `{"mode":"regex"}` listed
// as an effective rule at exit 0; it now aborts with exit code 3 naming the
// file and the field, as a blank mode already did.
func TestRulesListRefusesADetectBlockWithoutAPattern(t *testing.T) {
	for _, c := range []struct{ name, detect string }{
		{name: "absent", detect: `{"mode":"regex"}`},
		{name: "empty", detect: `{"mode":"regex","pattern":""}`},
		{name: "null", detect: `{"mode":"regex","pattern":null}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			layout := layeredHome(t)
			path := layout.Rule("no-pattern")
			body := `{"id":"no-pattern","title":"The no-pattern standard.","rationale":"Why no-pattern exists.",` +
				`"class":"sql-injection","detect":` + c.detect + `}`
			require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

			printed, err := runCLIPrinting(t, "rules", "list", "--repo", harvestSlug)

			var malformed *rule.MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, &rule.MalformedError{
				File:  path,
				Field: "detect.pattern",
				Problem: fmt.Sprintf(
					"of rule %q is absent or empty, and an empty regular expression matches every changed line",
					"no-pattern",
				),
			}, malformed)
			assert.Equal(t, ExitFile, exitCodeFor(err))
			assert.Equal(t, "repair the rule file the message names: the key it names, or the file "+
				"itself when it cannot be read; §2.6's table is the whole of what a rule may carry", hintFor(err))
			assert.Empty(t, printed, "a refused corpus lists no rule as effective")
		})
	}
}
