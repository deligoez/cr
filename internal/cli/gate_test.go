package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// qualityGate returns the gate command tp runs at `tp done`.
func qualityGate(t *testing.T) string {
	t.Helper()

	var file struct {
		Workflow struct {
			QualityGate string `json:"quality_gate"`
		} `json:"workflow"`
	}
	require.NoError(t, json.Unmarshal(repoFile(t, "spec/0.1.0.tasks.json"), &file))
	require.NotEmpty(t, file.Workflow.QualityGate, "the task file no longer declares a quality gate")
	return file.Workflow.QualityGate
}

// The quality gate is tp configuration, not code, so nothing in the compiler or
// the linter notices when a step is dropped from it. A gate step that can
// disappear without a test failing is exactly the blindness the step was added
// to close, so each one is named here.
//
// deadcode is the reason this test exists: golangci-lint's `unused` skips
// exported identifiers by design, so an exported function with zero callers
// passes `golangci-lint run`. Only the call graph notices, and only if the gate
// still runs it.
func TestQualityGateRunsEveryStep(t *testing.T) {
	gate := qualityGate(t)
	for _, step := range []string{
		"go test -race ./...",
		"golangci-lint run",
		"./scripts/deadcode.sh",
	} {
		assert.Contains(t, gate, step, "quality gate no longer runs this step")
	}
}

// A gate is only as strong as the weakest place it runs. `tp done` runs it on a
// developer's machine, but nothing made the two automated paths agree: the CI
// workflow ran `go test ./...` and golangci-lint — no -race, no deadcode — and
// the GoReleaser before-hooks ran `go test ./...` alone, so a change that failed
// the gate locally passed on the pull request that carried it and could be cut
// into a release. That is the same blindness the deadcode step was added to
// close, in two more places.
//
// The steps are derived from the gate string rather than restated, so a future
// gate change fails here until all three places carry it.
func TestEveryGateStepRunsInCIAndAtRelease(t *testing.T) {
	steps := strings.Split(qualityGate(t), "&&")
	require.NotEmpty(t, steps)

	ci := string(repoFile(t, ".github/workflows/ci.yml"))
	hooks := strings.Join(readGoreleaser(t).Before.Hooks, "\n")

	for _, step := range steps {
		step = strings.TrimSpace(step)
		assert.Contains(t, ci, step, "the CI workflow does not run this gate step")
		assert.Contains(t, hooks, step, "the GoReleaser before-hooks do not run this gate step")
	}
}
