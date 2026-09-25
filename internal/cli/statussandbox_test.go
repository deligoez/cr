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

// `cr status` reports the pull request's sandbox when it exists — its path,
// its size in bytes and its age — in the document and on one line of text, and
// reports nothing about one that is not there.
func TestStatusReportsTheSandboxOnDisk(t *testing.T) {
	stateHome(t, "OPEN")
	sandboxPath := state.New(crHomeOf(t)).Sandbox(fixtureOwner, fixtureProject, fixturePRNumber)
	status := func() map[string]any {
		t.Helper()
		printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
		require.NoError(t, err)
		var document map[string]any
		require.NoError(t, json.Unmarshal([]byte(printed), &document))
		return document
	}
	assert.NotContains(t, status(), "sandbox", "the control: no sandbox, no field")

	require.NoError(t, os.MkdirAll(sandboxPath, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(sandboxPath, ".env"), []byte("SECRET=x\n"), 0o600))
	reported, ok := status()["sandbox"].(map[string]any)
	require.True(t, ok, "the sandbox is reported")
	assert.Equal(t, sandboxPath, reported["path"])
	assert.InDelta(t, 9, reported["bytes"], 0)
	assert.Contains(t, reported, "since")
	assert.Contains(t, reported, "age_seconds")

	text := strings.ReplaceAll(throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--no-color"), "\r\n", "\n")
	assert.Contains(t, text, "\nsandbox: "+sandboxPath+", 9 bytes, built ")
}
