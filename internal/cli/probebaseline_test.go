package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Field feedback 1.7 through `cr probe run`: a probe whose result is the one its
// kind's section reads — a mutation's `no-test-failed`, a gap probe's `failed` —
// over a baseline that did not pass per §5.2.5 says so under `honesty`, naming
// the baseline run and its failed count. On a suite that already fails, the
// document otherwise reads `establishes: nothing` beside a `no-test-failed` and
// says nothing about why.
//
// Both directions are driven: the same result over a baseline that passed, and
// a different result over a baseline that failed, carry no such line, because
// the baseline changed nothing about what they establish. The sandbox is created
// first, so `honesty` holds this disclosure alone and is asserted whole.
func TestAProbeOverABaselineThatDidNotPassSaysSo(t *testing.T) {
	const (
		redSuite    = "echo 'Tests:  3 failed'\nexit 1\n"
		mutatedPass = "if grep -q 'func Retry() {}' app.go; then\n  echo 'Tests:  4 passed'\n  exit 0\nfi\n"
		probeFails  = "if [ -f " + gapProbePath + " ]; then\n  echo 'Tests:  1 failed'\n  exit 1\nfi\n"
	)
	for _, tc := range []struct {
		name, runner, kind, result string
		honesty                    []any
	}{
		{
			name: "a mutation no test noticed, over a red suite", kind: "mutation", result: "no-test-failed",
			runner: mutatedPass + redSuite,
			honesty: []any{"probe p1 establishes no gap: its baseline run r1 did not pass per §5.2.5 " +
				"(tests_failed 3), and §5.3.5 lets no-test-failed prove a gap only over a baseline that passed"},
		},
		{
			name: "a mutation no test noticed, over a baseline with no count", kind: "mutation",
			result: "no-test-failed",
			runner: mutatedPass + "echo 'the runner printed no recap'\nexit 1\n",
			honesty: []any{"probe p1 establishes no gap: its baseline run r1 did not pass per §5.2.5 " +
				"(no tests_failed was derivable), and §5.3.5 lets no-test-failed prove a gap only over a " +
				"baseline that passed"},
		},
		{
			name: "a mutation no test noticed, over a passing baseline", kind: "mutation", result: "no-test-failed",
			runner:  "echo 'Tests:  4 passed'\n",
			honesty: []any{},
		},
		{
			name: "a mutation the suite noticed, over a red suite", kind: "mutation", result: "failed",
			runner:  "if grep -q 'func Retry() {}' app.go; then\n  echo 'Tests:  4 failed'\n  exit 1\nfi\n" + redSuite,
			honesty: []any{},
		},
		{
			name: "a failed gap probe, over a red suite", kind: "gap", result: "failed",
			runner: probeFails + redSuite,
			honesty: []any{"probe p1 supports no finding: its baseline run r1 did not pass per §5.2.5 " +
				"(tests_failed 3), and §5.4.4 lets a failed gap probe support a finding only over a " +
				"baseline that passed"},
		},
		{
			name: "a failed gap probe, over a passing baseline", kind: "gap", result: "failed",
			runner:  probeFails + "echo 'Tests:  5 passed'\n",
			honesty: []any{},
		},
		{
			name: "a passing gap probe, over a red suite", kind: "gap", result: "passed",
			runner:  "if [ -f " + gapProbePath + " ]; then\n  echo 'Tests:  5 passed'\n  exit 0\nfi\n" + redSuite,
			honesty: []any{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probeFixture(t, tc.runner, gapProbeTemplate)
			require.NoError(t, runCLI(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug))

			var shown map[string]any
			if tc.kind == "gap" {
				shown = runGap(t, writeProbeTest(t))
			} else {
				shown = runProbe(t, writePatch(t, fixtureDiff))
			}
			require.Equal(t, tc.result, shown["result"], "the runner produced the case under test")
			assert.Equal(t, "r1", shown["baseline"])
			assert.Equal(t, tc.honesty, shown["honesty"])
		})
	}
}

// The baseline a second probe at the same head resolves from runs.ndjson,
// rather than performs, is named the same way: the disclosure reads the stored
// record the probe points at, not only a run this invocation made.
func TestASecondProbeOverTheStoredRedBaselineSaysSo(t *testing.T) {
	probeFixture(t, "if grep -q 'func Retry() {}' app.go; then\n  echo 'Tests:  4 passed'\n  exit 0\nfi\n"+
		"echo 'Tests:  2 failed'\nexit 1\n")
	require.NoError(t, runCLI(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug))
	patch := writePatch(t, fixtureDiff)
	runProbe(t, patch)

	second := runProbe(t, patch)
	require.Equal(t, "p2", second["probe"])
	assert.Equal(t, "r1", second["baseline"], "§5.2.6: the stored baseline is resolved, not performed again")
	assert.Equal(t, []any{"probe p2 establishes no gap: its baseline run r1 did not pass per §5.2.5 " +
		"(tests_failed 2), and §5.3.5 lets no-test-failed prove a gap only over a baseline that passed"},
		second["honesty"])
}
