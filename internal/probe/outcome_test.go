package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// §5.1.7: a probe whose post-run cleanliness check fails is recorded with
// result `error`, "overriding the ladder outcome".
//
// Every value §5.3.4's and §5.4.3's ladders can produce is driven through, and
// the two that matter most are the two that would otherwise say something. A
// mutation probe's `no-test-failed` is the only result §5.3.5 lets prove a gap,
// and `failed` is the one §5.3.7 lets disprove it — and §5.1.7 says such a
// probe "establishes nothing in either direction", so neither value survives
// the override. §5.3.7's suppression is then not read from the record because
// the value it keys on is not the value that was written.
func TestAnUncleanSandboxOverridesEveryLadderOutcome(t *testing.T) {
	for _, ladder := range []Result{
		"no-test-failed", "failed", "passed", "no-tests-selected",
		"inconclusive", "timeout", ResultError,
	} {
		t.Run(string(ladder), func(t *testing.T) {
			clean := Decide(ladder, "")
			assert.Equal(t, ladder, clean.Result(),
				"a sandbox that passed the check leaves the ladder's answer alone")
			assert.False(t, clean.Voided())

			voided := Decide(ladder, "a probe artefact was left behind: cr_probe_p1.txt")
			assert.Equal(t, ResultError, voided.Result(),
				"§5.1.7: the check overrides the ladder outcome")
			assert.True(t, voided.Voided(),
				"§5.1.7: the probe grades no finding and the sandbox is recreated")
		})
	}
}
