package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// sandboxGeneration reads the generation the sandbox's post-setup baseline
// names.
func sandboxGeneration(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(state.New(crHomeOf(t)).PRFile(
		fixtureOwner, fixtureProject, fixturePRNumber, state.FileSandboxBaseline))
	require.NoError(t, err)
	var recorded map[string]any
	require.NoError(t, json.Unmarshal(body, &recorded))
	generation, ok := recorded["generation"].(string)
	require.True(t, ok, "the post-setup baseline names a generation: %s", body)
	return generation
}

// crHomeOf is the state root the test's CR_HOME points at.
func crHomeOf(t *testing.T) string {
	t.Helper()
	home := os.Getenv(state.HomeEnv)
	require.NotEmpty(t, home)
	return home
}

// A baseline measured in a sandbox §5.1.6 has since rebuilt is not the
// baseline of a probe in the rebuilt one: the second probe performs both of
// §5.2.2's baselines again, and its `baseline` names the new filtered run
// rather than the passing one recorded before the recreation.
func TestABaselineFromAnEarlierSandboxIsNotReusedAfterARecreation(t *testing.T) {
	_, sandboxPath, runner, _ := envFixture(t, []string{".env"}, ".env")
	probing := []string{"probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", writePatch(t, fixtureDiff), "--filter", "retries"}

	stdout, _ := streams(t, probing...)
	var first map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &first))
	require.Equal(t, "r2", first["baseline"], "the control: the first probe performed r1 and r2, and ran as r3")
	before := sandboxGeneration(t)

	require.NoError(t, os.WriteFile(filepath.Join(checkout(t), ".env"), []byte("SECRET=changed-now\n"), 0o600))
	stdout, stderr := streams(t, probing...)
	var second map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &second))
	assert.Equal(t, "r5", second["baseline"], "§5.2.6 performed the baselines again, as r4 and r5")
	assert.Equal(t, "r6", second["run"])
	assert.Equal(t, 3, strings.Count(afterHeader(t, stderr), recapLine),
		"the unfiltered baseline, the filtered one, then the probe")
	assert.Contains(t, strings.Split(stderr, "\n"),
		"  baseline   the whole suite runs first as the §5.2.2 baseline: "+runner)
	assert.Equal(t, []any{"sandbox " + sandboxPath + " recreated, per §5.1.6: sandbox.copy in " +
		state.New(crHomeOf(t)).Profile("qa") + " names .env, and the checkout " + checkout(t) +
		" holds a copy that differs from the sandbox's"}, honestyList(t, stdout))

	after := sandboxGeneration(t)
	require.NotEqual(t, before, after, "the recreation recorded a new generation")
	stamped := map[string]any{}
	for _, record := range storedRecords(t, state.New(crHomeOf(t)), state.FileRuns) {
		stamped[record["id"].(string)] = record["sandbox"]
	}
	assert.Equal(t, map[string]any{
		"r1": before, "r2": before, "r3": before,
		"r4": after, "r5": after, "r6": after,
	}, stamped, "every run is stamped with the sandbox it measured")
}

// A sandbox whose post-setup baseline names no generation — one an earlier cr
// created — is recreated before the run with the reason said, and the next
// run finds nothing to recreate.
func TestASandboxWithoutAGenerationIsRecreated(t *testing.T) {
	_, sandboxPath, _, _ := envFixture(t, []string{".env"}, ".env")
	streams(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
	file := state.New(crHomeOf(t)).PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileSandboxBaseline)
	body, err := os.ReadFile(file)
	require.NoError(t, err)
	var recorded map[string]any
	require.NoError(t, json.Unmarshal(body, &recorded))
	delete(recorded, "generation")
	legacy, err := json.Marshal(recorded)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, legacy, 0o600))

	stdout, _ := streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, []any{"sandbox " + sandboxPath + " recreated, per §5.1.6: " +
		"its post-setup baseline names no sandbox generation, so no run can be tied to it"}, honestyList(t, stdout))
	stdout, _ = streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, []any{}, honestyList(t, stdout))
}
