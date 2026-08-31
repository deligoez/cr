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
