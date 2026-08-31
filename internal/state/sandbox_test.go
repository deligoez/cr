package state

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §5.1.1 puts the sandbox at ~/.cr/state/<owner>/<repo>/pr-<n>/sandbox/, which
// is one directory inside the pull request's own state directory of §2.3.
//
// The path is asserted whole rather than as a join onto PRDir, because the two
// halves fail differently: a sandbox under the wrong pull request would still
// sit under PRDir, and a sandbox under some sibling of the state root would
// still end in `sandbox`. §5.1.1 fixes both halves and this reads both.
func TestTheSandboxSitsInThePullRequestsStateDirectory(t *testing.T) {
	root := filepath.Join("home", ".cr")
	l := New(root)

	assert.Equal(t,
		filepath.Join(root, "state", "acme", "web", "pr-42", "sandbox"),
		l.Sandbox("acme", "web", 42))
	assert.Equal(t, filepath.Join(l.PRDir("acme", "web", 42), DirSandbox),
		l.Sandbox("acme", "web", 42))
}

// §5.1.2: a copied path arrives in the sandbox as it stands in the checkout.
//
// The three shapes are asserted together because each fails on its own. A file
// carries a mode, and a `vendor/bin` entry that lost its executable bit is a
// suite that cannot run; a directory has to arrive whole rather than as its top
// level; and a symlink has to stay one, since `vendor` is full of them and
// following them would copy the target instead of the link.
func TestCopyIntoSandboxReproducesFilesDirectoriesAndLinks(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), ".cr"))
	checkout := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(checkout, ".env"), []byte("APP_ENV=testing\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(checkout, "vendor", "bin"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(checkout, "vendor", "bin", "pest"), []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.Symlink(
		filepath.Join("..", "bin", "pest"), filepath.Join(checkout, "vendor", "pest-link")))

	for _, rel := range []string{".env", "vendor"} {
		copied, err := l.CopyIntoSandbox("acme", "web", 42, checkout, rel)
		require.NoError(t, err)
		assert.True(t, copied, "%s is in the checkout", rel)
	}

	sandbox := l.Sandbox("acme", "web", 42)
	body, err := os.ReadFile(filepath.Join(sandbox, ".env"))
	require.NoError(t, err)
	assert.Equal(t, "APP_ENV=testing\n", string(body))

	runner, err := os.Lstat(filepath.Join(sandbox, "vendor", "bin", "pest"))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o755), runner.Mode().Perm(),
		"a copied runner that lost its executable bit is a suite that cannot run")

	pointsAt, err := os.Readlink(filepath.Join(sandbox, "vendor", "pest-link"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("..", "bin", "pest"), pointsAt,
		"§5.1.2 copies the path, and a link followed instead of copied is a different tree")
}

// A `sandbox.copy` path the checkout does not hold is reported, not refused.
//
// The laravel-pest profile lists `vendor`, and a fresh clone has none: it is
// what §5.1.3's `composer install` is there to create. A copy that failed on
// the absent path would stop the run one step before the step that fixes it,
// so what comes back is the answer rather than an error — and nothing is
// created for a path that was never there, which is the half that would
// otherwise leave an empty directory standing in for a missing tree.
func TestCopyIntoSandboxReportsAPathTheCheckoutDoesNotHold(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), ".cr"))

	copied, err := l.CopyIntoSandbox("acme", "web", 42, t.TempDir(), "vendor")

	require.NoError(t, err)
	assert.False(t, copied)
	assert.NoFileExists(t, filepath.Join(l.Sandbox("acme", "web", 42), "vendor"))
	assert.NoDirExists(t, filepath.Join(l.Sandbox("acme", "web", 42), "vendor"))
}

// A copy replaces what the sandbox already holds at that path.
//
// The sandbox is a checkout of the head, so a `sandbox.copy` entry naming a
// path the repository also tracks arrives on top of a file git just wrote —
// and a symlink is the case that cannot simply be overwritten, since one
// cannot be created over an existing name. §5.1.2 copies the checkout's
// version, so what stands there afterwards is the checkout's and not a mixture
// of the two.
func TestCopyIntoSandboxReplacesWhatTheSandboxAlreadyHolds(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), ".cr"))
	checkout := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(checkout, "vendor"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(checkout, "vendor", "autoload.php"), []byte("the checkout's\n"), 0o600))
	require.NoError(t, os.Symlink("autoload.php", filepath.Join(checkout, "vendor", "link")))

	// What the worktree checkout left standing: a file with other
	// contents, and a link pointing somewhere else entirely.
	sandbox := l.Sandbox("acme", "web", 42)
	require.NoError(t, os.MkdirAll(filepath.Join(sandbox, "vendor"), 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(sandbox, "vendor", "autoload.php"), []byte("the worktree's\n"), 0o600))
	require.NoError(t, os.Symlink("somewhere-else", filepath.Join(sandbox, "vendor", "link")))

	copied, err := l.CopyIntoSandbox("acme", "web", 42, checkout, "vendor")
	require.NoError(t, err)
	assert.True(t, copied)

	body, err := os.ReadFile(filepath.Join(sandbox, "vendor", "autoload.php"))
	require.NoError(t, err)
	assert.Equal(t, "the checkout's\n", string(body))

	pointsAt, err := os.Readlink(filepath.Join(sandbox, "vendor", "link"))
	require.NoError(t, err)
	assert.Equal(t, "autoload.php", pointsAt)
}
