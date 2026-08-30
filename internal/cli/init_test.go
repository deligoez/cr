package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/role"
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

// runInit runs `cr init` with the given flags through the real command tree,
// so what a test measures is what a user would get.
func runInit(t *testing.T, args ...string) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"init"}, args...))
	require.NoError(t, cmd.Execute())
}

// §2.5.2 requires `cr init --eject-roles` to write the defaults as editable
// files that are byte-identical to the built-ins. Byte-identical is the whole
// of it: the file a user is invited to edit has to start as the reviewed file
// this repository ships, not as whatever an encoder would produce from it.
//
// The second eject is the other half of §2.5.2's word `editable`. Re-running
// the flag has to complete a tree without reverting an edit, because cr still
// holds the default in the binary while the user holds the only copy of what
// they wrote.
func TestEjectRolesWritesTheBuiltinsByteForByte(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".cr")
	t.Setenv(state.HomeEnv, root)
	ejected := func(t *testing.T, id string) string {
		t.Helper()
		onDisk, err := os.ReadFile(filepath.Join(root, "roles", id+".json"))
		require.NoError(t, err)
		return string(onDisk)
	}

	runInit(t, "--eject-roles")

	shipped := role.Builtins()
	require.Contains(t, shipped, "correctness", "§2.5.1 fixes which four roles ship")
	for id, content := range shipped {
		assert.Equal(t, content, ejected(t, id))
	}

	const edit = `{"id": "correctness"}`
	mine := filepath.Join(root, "roles", "correctness.json")
	require.NoError(t, os.WriteFile(mine, []byte(edit), 0o600))
	runInit(t, "--eject-roles")

	assert.Equal(t, edit, ejected(t, "correctness"), "a second eject reverts no edit")
	for id, content := range shipped {
		if id == "correctness" {
			continue
		}
		assert.Equal(t, content, ejected(t, id), "and leaves the rest as they were")
	}
}

// §2.5.2 puts the eject behind a flag, and §2.5.4 says why that has to hold:
// an unejected role is read from the built-in layer, so a `cr init` that wrote
// the four files anyway would leave every tree carrying a frozen copy of
// defaults nobody asked for — one that no upgrade could ever refresh, because
// §2.5.2 also forbids overwriting what the user may have edited.
func TestInitWritesNoRoleWithoutTheEjectFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".cr")
	t.Setenv(state.HomeEnv, root)

	runInit(t)

	entries, err := os.ReadDir(filepath.Join(root, "roles"))
	require.NoError(t, err, "§2.2 lays out the directory either way")
	assert.Empty(t, entries)
}
