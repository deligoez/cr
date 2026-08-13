package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	path := filepath.Join("..", "..", "spec", "0.1.0.tasks.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "the task file holds the gate; if it moved, move this test with it")

	var file struct {
		Workflow struct {
			QualityGate string `json:"quality_gate"`
		} `json:"workflow"`
	}
	require.NoError(t, json.Unmarshal(raw, &file))

	gate := file.Workflow.QualityGate
	for _, step := range []string{
		"go test ./...",
		"golangci-lint run",
		"./scripts/deadcode.sh",
	} {
		assert.Contains(t, gate, step, "quality gate no longer runs this step")
	}
}
