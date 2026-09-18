package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §5.1.5: the worktree and its registration both go, and §5.1.6's baseline
// goes with them.
//
// The baseline is the part worth asserting here rather than in the command.
// Create invalidates it before adding a worktree, because between the two
// moments there is a checkout nothing has measured; a destruction leaves the
// same situation pointed the other way, and a baseline left behind would
// describe tracked files that no longer exist anywhere.
func TestDestroyRemovesTheWorktreeItsRegistrationAndTheBaseline(t *testing.T) {
	dir, head := repository(t)
	src := sources(t, dir, head)

	created, err := Create(src)
	require.NoError(t, err)
	require.DirExists(t, created.Path)
	baseline := src.Layout.PRFile(fixtureOwner, fixtureRepo, fixturePR, state.FileSandboxBaseline)
	require.FileExists(t, baseline, "the creation recorded §5.1.6's baseline")

	removed, err := Destroy(src)
	require.NoError(t, err)
	assert.Equal(t, created.Path, removed.Path)
	assert.True(t, removed.Existed, "there was a worktree to remove")

	assert.NoDirExists(t, created.Path)
	assert.NoDirExists(t, filepath.Join(dir, ".git", "worktrees", "sandbox"),
		"§5.1.5: the registration goes with the worktree")
	assert.NoFileExists(t, baseline,
		"§5.1.6's baseline described the sandbox that has just been removed")
}

// §5.1.5 asks for the worktree to be gone, and a pull request that never had
// one is already in that state — so the removal reports it rather than failing.
//
// It still prunes, and that is the case it exists for. A directory somebody
// deleted by hand leaves the registration behind, and `worktree add` refuses a
// path that is still registered: the recreation §5.1.6 mandates would fail on
// the one path cr is entitled to.
func TestDestroyPrunesARegistrationWhoseDirectoryIsAlreadyGone(t *testing.T) {
	dir, head := repository(t)
	src := sources(t, dir, head)

	created, err := Create(src)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(created.Path))

	removed, err := Destroy(src)
	require.NoError(t, err)
	assert.False(t, removed.Existed, "there was no worktree at that path")
	assert.NoDirExists(t, filepath.Join(dir, ".git", "worktrees", "sandbox"),
		"the registration the deleted directory left behind is pruned")

	// The proof that the prune mattered: §5.1.1 can now use the path again.
	again, err := Create(src)
	require.NoError(t, err, "a stranded registration would refuse the path here")
	assert.Equal(t, created.Path, again.Path)
}

// A sandbox whose registration is gone is still destroyed, and §5.1.1 can
// create over the path afterwards.
//
// This is what re-cloning the repository under review leaves: the directory
// under ~/.cr survives and `.git/worktrees/` does not, so `worktree remove`
// refuses the directory as not a working tree while Create refuses to build
// over it. Measured before the fallback existed, destroy exited 3 and create
// exited 4 on that state, with nothing in cr able to clear it.
func TestDestroyRemovesASandboxWhoseRegistrationIsGone(t *testing.T) {
	dir, head := repository(t)
	src := sources(t, dir, head)

	created, err := Create(src)
	require.NoError(t, err)
	registration := filepath.Join(dir, ".git", "worktrees", "sandbox")
	require.DirExists(t, registration)
	require.NoError(t, os.RemoveAll(registration), "the registration removed behind cr's back")

	removed, err := Destroy(src)
	require.NoError(t, err, "§5.1.5 has no state it cannot clear")
	assert.True(t, removed.Existed, "there was a directory to remove")
	assert.NoDirExists(t, created.Path)
	assert.NoDirExists(t, registration)

	again, err := Create(src)
	require.NoError(t, err, "§5.1.1 refuses a path whose directory is still there")
	assert.Equal(t, created.Path, again.Path)
}

// A sandbox whose registration is gone and whose directory cannot be deleted is
// a destruction that failed, and it is reported as one. The directory is still
// there, so an answer of success would tell the reader §5.1.5 holds while
// §5.1.1 goes on refusing the path.
func TestAnOrphanedSandboxThatCannotBeRemovedFailsTheDestruction(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root deletes through a read-only directory")
	}
	dir, head := repository(t)
	src := sources(t, dir, head)

	created, err := Create(src)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".git", "worktrees", "sandbox")),
		"the registration removed behind cr's back")
	locked := filepath.Join(created.Path, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(locked, "held"), []byte("x"), 0o600))
	require.NoError(t, os.Chmod(locked, 0o500))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	removed, err := Destroy(src)

	require.Error(t, err, "§5.1.5: the sandbox directory is still there")
	assert.Nil(t, removed)
	assert.DirExists(t, created.Path)
}
