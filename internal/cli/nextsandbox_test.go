package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// nextSteps runs `cr next` and returns its steps.
func nextSteps(t *testing.T) []nextStep {
	t.Helper()
	printed, err := runCLIPrinting(t, "next", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var result nextResult
	require.NoError(t, json.Unmarshal([]byte(printed), &result))
	return result.Steps
}

// §10.4.10: when the round's last `cr brief` found the pull request not open
// and its sandbox exists, `cr next` reports a `sandbox` step after the others,
// taken by the agent, naming `cr sandbox destroy` and the state; with the
// sandbox gone, the step goes with it.
func TestNextNamesTheSandboxAMergedPullRequestLeaves(t *testing.T) {
	stateHome(t, gh.StateMerged)
	sandboxPath := state.New(crHomeOf(t)).Sandbox(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, os.MkdirAll(sandboxPath, 0o700))

	steps := nextSteps(t)
	require.NotEmpty(t, steps)
	assert.Equal(t, nextStep{
		Step: "sandbox", Actor: actorAgent,
		Why: "the last `cr brief` found the pull request merged at " + closedAt + ", and its sandbox still holds a " +
			"checkout and the copies of its gitignored files; destroy it once no run is owed in it",
		Commands: []string{"cr sandbox destroy " + fixturePR + " --repo " + fixtureSlug},
		Items:    []string{sandboxPath},
	}, steps[len(steps)-1], "the step comes after the others")

	require.NoError(t, os.RemoveAll(sandboxPath))
	for _, step := range nextSteps(t) {
		assert.NotEqual(t, "sandbox", step.Step, "no sandbox, no step")
	}
}

// The control: an open pull request's sandbox is the one its runs need, and
// `cr next` owes no step for it.
func TestNextOwesNoSandboxStepForAnOpenPullRequest(t *testing.T) {
	stateHome(t, "OPEN")
	require.NoError(t, os.MkdirAll(
		state.New(crHomeOf(t)).Sandbox(fixtureOwner, fixtureProject, fixturePRNumber), 0o700))
	for _, step := range nextSteps(t) {
		assert.NotEqual(t, "sandbox", step.Step)
	}
}
