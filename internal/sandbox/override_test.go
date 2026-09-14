package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/probe"
)

// §5.1.7 end to end: the post-run cleanliness check runs before the probe
// record is written, a probe whose check fails is recorded with result `error`
// overriding the ladder outcome, and the sandbox is recreated before the next
// run.
//
// The ladder is given `no-test-failed`, which is the one mutation result §5.3.5
// lets prove a gap, so the override is the difference between a record that
// licenses a `probed` assertion to a colleague and one that licenses nothing.
//
// Exactly one value reaches the record, and that is round 9's
// probe-result-override finding: the check comes first, so §5.5.1's
// immutability is never in tension with §5.3.4's totality — there is no
// already-written record to amend.
func TestAProbeWhoseSandboxFailedItsCheckIsRecordedErrorAndForcesRecreation(t *testing.T) {
	src, path, sentinel := sandboxed(t)

	// What a probe that failed to remove its test file leaves behind, which
	// is one of the two things §5.1.6 calls unclean.
	artefact := filepath.Join(path, "cr_probe_p1.txt")
	require.NoError(t, os.WriteFile(artefact, []byte("the probe's own test\n"), 0o600))

	reason, err := Unclean(src, fixtureLeftoverGlob)
	require.NoError(t, err)
	require.Contains(t, reason, "cr_probe_p1.txt",
		"§5.1.6: the post-run check finds the leftover artefact")

	// The one write, taken after the check and never before it.
	var recorded []probe.Result
	outcome := probe.Decide("no-test-failed", reason)
	recorded = append(recorded, outcome.Result())

	require.Len(t, recorded, 1, "§5.5.1: one record is written, and it is never amended")
	assert.Equal(t, probe.ResultError, recorded[0],
		"§5.1.7: the check overrides what §5.3.4's ladder produced")
	require.True(t, outcome.Voided(), "§5.1.7: the probe grades no finding")

	require.NoError(t, ForceRecreation(src, "probe p1 was voided after its run: "+reason))

	ready, err := Ensure(src, fixtureLeftoverGlob)
	require.NoError(t, err)
	require.NotNil(t, ready.Recreated, "§5.1.7: recreation is forced before the next run")
	assert.Equal(t, "§5.1.7 forced its recreation: probe p1 was voided after its run: "+reason,
		ready.Recreated.Reason,
		"the forcing is what recreated it, and the notice names what the probe left behind")
	assert.NoFileExists(t, artefact, "the rebuilt sandbox is a fresh checkout")
	assert.NoFileExists(t, sentinel, "and the recreation really happened")
}
