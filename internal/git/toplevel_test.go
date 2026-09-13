package git

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Toplevel answers the repository root from the root and from a directory
// below it alike, which is what lets §5.6.1's lock mean one repository wherever
// cr was started.
//
// The root is compared as a whole value against the fixture's own path with
// symbolic links resolved, because a temporary directory is commonly reached
// through one and git reports the resolved path.
func TestToplevelIsTheRepositoryRootFromAnyDirectoryInIt(t *testing.T) {
	repo := fixtureRepo(t)
	nested := filepath.Join(repo, "internal", "deeper")
	require.NoError(t, os.MkdirAll(nested, 0o750))
	root, err := filepath.EvalSymlinks(repo)
	require.NoError(t, err)

	for _, from := range []string{repo, filepath.Join(repo, "internal"), nested} {
		at, err := Toplevel(from)
		require.NoError(t, err)
		assert.Equal(t, root, at, "run from %s", from)
	}
}

// A root whose name ends in a space keeps it: only git's own line ending is
// removed from the answer.
func TestToplevelKeepsATrailingSpaceInTheRootsName(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo := filepath.Join(base, "checkout ")
	require.NoError(t, os.Mkdir(repo, 0o750))
	fixtureGit(t, repo, "init", "--quiet", "--initial-branch=main")

	at, err := Toplevel(repo)

	require.NoError(t, err)
	assert.Equal(t, repo, at)
}

// A directory in no repository has no root, and the refusal is git's own
// CommandError, which §3.1.3 codes like every failing external command.
func TestToplevelOutsideARepositoryIsACommandError(t *testing.T) {
	_, err := Toplevel(t.TempDir())

	var failed *CommandError
	require.True(t, errors.As(err, &failed), "got %v", err)
}
