package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/deligoez/cr/internal/profile"
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

// §11.1 gives `cr init` the job of writing the default profiles. Two things
// must hold of what it writes. The bytes must be the built-in's own — §2.5.2
// asks that of the ejected roles, and a profile the user is invited to edit is
// worth no less — and the file must load, because a profile cr ships and then
// refuses would abort every command that reads it with exit code 3.
//
// The second run is the other half: a profile is the user's to edit, so init
// must be safe to re-run against a tree they have changed.
func TestInitWritesTheShippedProfiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".cr")
	t.Setenv(state.HomeEnv, root)
	run := func(t *testing.T) {
		t.Helper()
		cmd := newRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetArgs([]string{"init"})
		require.NoError(t, cmd.Execute())
	}

	run(t)

	shipped := profile.Builtins()
	require.Contains(t, shipped, "laravel-pest")
	for id, content := range shipped {
		written := filepath.Join(root, "profiles", id+".json")
		onDisk, err := os.ReadFile(written)
		require.NoError(t, err)
		assert.Equal(t, content, string(onDisk))

		p, err := profile.Load(written)
		require.NoError(t, err)
		assert.Equal(t, id, p.ID)
	}

	edited := filepath.Join(root, "profiles", "laravel-pest.json")
	require.NoError(t, os.WriteFile(edited, []byte(`{"id": "laravel-pest"}`), 0o600))
	run(t)
	onDisk, err := os.ReadFile(edited)
	require.NoError(t, err)
	assert.Equal(t, `{"id": "laravel-pest"}`, string(onDisk))
}
