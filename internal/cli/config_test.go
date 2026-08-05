package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cr config prints the configuration that is actually in force, so the layers
// of spec/0.1.0.md §2.7 are inspectable rather than inferred.
func TestConfigCommandPrintsTheEffectiveConfiguration(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".cr")
	t.Setenv(state.HomeEnv, root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "repos", "acme", "web"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"),
		[]byte(`{"post": {"max_comments": 9}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "repos", "acme", "web", "config.json"),
		[]byte(`{"post": {"max_comments": 7}}`), 0o600))

	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"config", "--repo", "acme/web"})
	require.NoError(t, cmd.Execute())

	var printed map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &printed))
	assert.Equal(t, float64(7), printed["post.max_comments"])
	assert.Equal(t, "tr", printed["render.lang"])
	assert.Equal(t, []any{}, printed["ignore.globs"])
}
