package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
)

// testLensSection is what every test-axis prompt says about what the lens can
// run and what a classification means on a unit that is itself a test.
const testLensSection = "## What this lens can run (§4.4, §5.7)\n\n" +
	"You cannot run tests. The only way to show a gap by experiment is a §5.7 proposal, below, " +
	"which `cr probe run --proposal <id>` runs.\n\n" +
	"When this unit is itself a test file, its cell's classification says whether that test's own " +
	"assertions exercise the behaviour it claims to test; where that says nothing, `na` with a reason " +
	"is the right cell.\n"

// Through `cr review`: a test-adequacy prompt says plainly that the role runs
// nothing and that a §5.7 proposal is its only experiment, and what its
// classification means on a unit holding a test file; no prompt of another
// axis carries it. The unit here is lib_test.go, which the pull request adds.
// Both came from the first live tester's report on tarfin-labs/backend#6328.
func TestATestAxisPromptSaysWhatTheLensCanRun(t *testing.T) {
	testSymbolsHome(t, false)

	prompts := fanoutOf(t).Prompts
	require.NotEmpty(t, prompts)
	tested := 0
	for _, prompt := range prompts {
		if prompt.Axis != axis.Test {
			assert.NotContains(t, prompt.Text, testLensSection, "%s is not on the test axis", prompt.Role)
			continue
		}
		tested++
		assert.Contains(t, prompt.Text, testLensSection)
	}
	assert.Equal(t, 1, tested, "one test-adequacy prompt over the one unit")
}
