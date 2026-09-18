package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/sandbox"
)

// survivorRunner passes and leaves a process of its own behind, holding the
// runner lock cr passed the runner as a descriptor.
//
// The sleep outlives state.SurvivorGrace on purpose: a shorter one would test
// the grace rather than the disclosure, and the point is a process that is
// genuinely still there when cr reports the run.
const survivorRunner = "sleep 5 &\necho 'Tests:  4 passed'\n"

// quietRunner passes and leaves nothing behind.
const quietRunner = "echo 'Tests:  4 passed'\n"

// honestyOfTestRun runs `cr test` over probeFixture's pull request and returns
// what it disclosed.
func honestyOfTestRun(t *testing.T) []string {
	t.Helper()
	printed, err := runCLIPrinting(t, "test", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report struct {
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	return report.Honesty
}

// A process the runner started that outlives the run is disclosed, because
// §5.2.3's kill reaches a process group and cr cannot signal what left it
// (§5.6.3).
func TestARunnerSurvivorIsDisclosedByTheRunThatLeftIt(t *testing.T) {
	probeFixture(t, survivorRunner)

	assert.Contains(t, honestyOfTestRun(t), sandbox.RunnerSurvivor{}.Disclosure(),
		"§5.6.3: the run says a process of its runner is still holding the sandbox")
}

// A run whose runner left nothing behind discloses nothing, so the notice
// means what it says rather than appearing on every run.
func TestARunThatLeftNoSurvivorDisclosesNone(t *testing.T) {
	probeFixture(t, quietRunner)

	assert.NotContains(t, honestyOfTestRun(t), sandbox.RunnerSurvivor{}.Disclosure())
}

// The probe path discloses it too: a straggler of a baseline is running beside
// the probe's own run, which is the reading §5.6.3's notice exists for.
func TestAProbeDisclosesARunnerSurvivor(t *testing.T) {
	probeFixture(t, survivorRunner)

	printed, err := runCLIPrinting(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", writePatch(t, fixtureDiff))
	require.NoError(t, err)
	var report struct {
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	assert.Contains(t, report.Honesty, sandbox.RunnerSurvivor{}.Disclosure())
}
