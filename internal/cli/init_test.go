package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitCommandCreatesTheStateTree(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".cr")
	t.Setenv(state.HomeEnv, root)

	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"init"})
	require.NoError(t, cmd.Execute())

	assert.DirExists(t, filepath.Join(root, "locks"))
	assert.FileExists(t, filepath.Join(root, "config.json"))
	assert.Contains(t, out.String(), root)
}
