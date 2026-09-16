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
// baseline of a probe in the rebuilt one: the second probe performs §5.2.2's
// baseline again, and its `baseline` names the new run rather than the passing
// one recorded before the recreation.
//
// §5.2.6's sentence is the one under test — "when no matching run exists at the
// current head and sandbox generation, cr MUST perform and record it before the
// probe" — and the head never moves here, so the generation is the only thing
// that could have made the stored run inadmissible.
func TestABaselineFromAnEarlierSandboxIsNotReusedAfterARecreation(t *testing.T) {
	_, sandboxPath, runner, _ := envFixture(t, []string{".env"}, ".env")
	probing := []string{"probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", writePatch(t, fixtureDiff), "--filter", "retries"}

	stdout, _ := streams(t, probing...)
	var first map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &first))
	require.Equal(t, "r1", first["baseline"], "the control: the first probe performed r1 and ran as r2")
	before := sandboxGeneration(t)

	require.NoError(t, os.WriteFile(filepath.Join(checkout(t), ".env"), []byte("SECRET=changed-now\n"), 0o600))
	stdout, stderr := streams(t, probing...)
	var second map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &second))
	assert.Equal(t, "r3", second["baseline"], "§5.2.6 performed the baseline again, as r3")
	assert.Equal(t, "r4", second["run"])
	assert.Equal(t, 2, strings.Count(afterHeader(t, stderr), recapLine),
		"§5.2.2's one baseline, then the probe")
	assert.Contains(t, strings.Split(stderr, "\n"),
		"  baseline   §5.2.2's baseline is not on file and runs first: "+runner+" --only retries")
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
		"r1": before, "r2": before,
		"r3": after, "r4": after,
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

// §5.2.6: "A run record carrying no `sandbox` matches no sandbox generation."
//
// The record below is one an earlier cr wrote: it stands at the round's head,
// carries no probe and passed, so every other condition §5.2.6 puts on a
// baseline holds — and the missing generation is the only thing that can keep
// it from being resolved as one. A probe that reused it would be graded against
// a sandbox nothing can identify, which is exactly what stamping the generation
// was added to prevent.
func TestARunFromBeforeGenerationsIsNoBaseline(t *testing.T) {
	prepared, fixture, _, _ := probeFixture(t, passingRunner)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))
	legacy := map[string]any{
		"id": "r1", "head": head, "round": 2, "exit_code": 0,
		"timed_out": false, "contaminated": false, "duration_ms": 1,
		"tests_run": 4, "tests_failed": 0, "output_tail": "Tests:  4 passed\n", "passed": true,
	}
	line, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		prepared.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileRuns),
		append(line, '\n'), 0o600))

	reported := probeDocument(t, throughAPipe(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", writePatch(t, fixtureDiff)))

	assert.Equal(t, "r2", reported["baseline"],
		"§5.2.6 performed a fresh baseline rather than resolving the record naming no generation")
	runs := storedRecords(t, prepared, state.FileRuns)
	require.Len(t, runs, 3, "the legacy record, the baseline performed for it, and the probe's own run")
	assert.NotContains(t, runs[0], "sandbox", "the record left in place still names no generation")
	assert.NotEmpty(t, runs[1]["sandbox"], "and the run performed instead names the sandbox it measured")
}
