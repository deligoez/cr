package state

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unopenable puts a lock file at path that nobody but root may open, so the
// flock fails at its open while every directory above it exists.
func unopenable(t *testing.T, path string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root opens a file whatever its mode")
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(path), dirPerm))
	require.NoError(t, os.WriteFile(path, nil, 0o000))
}

// §11.2 codes a lock that cannot be taken 3, as it does a write that cannot
// land: the command line was right and what refused was a file under the state
// root. Audit round 1 measured LockPR under a root whose locks path was a file
// exiting 2 with the usage hint, because the failure was a bare fmt.Errorf.
//
// Each case isolates one step of LockPR: a file where the owner's lock directory
// goes fails the directory creation, and a lock file nobody may open lets every
// directory be created and fails the flock alone.
func TestALockThatCannotBeTakenIsAFileFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		block func(t *testing.T, l Layout)
		path  func(l Layout) string
		hint  string
	}{
		"a file where the lock directory goes": {
			block: func(t *testing.T, l Layout) {
				require.NoError(t, os.WriteFile(filepath.Join(l.LocksDir(), "acme"), nil, filePerm))
			},
			path: func(l Layout) string { return filepath.Join(l.LocksDir(), "acme", "web") },
			hint: dirHint,
		},
		"a lock file nobody may open": {
			block: func(t *testing.T, l Layout) { unopenable(t, l.PRLockFile("acme", "web", 42)) },
			path:  func(l Layout) string { return l.PRLockFile("acme", "web", 42) },
			hint:  lockHint,
		},
	} {
		t.Run(name, func(t *testing.T) {
			l := New(filepath.Join(t.TempDir(), ".cr"))
			require.NoError(t, l.Init())
			tc.block(t, l)

			held, err := l.LockPR("acme", "web", 42)

			assert.Nil(t, held)
			var file *FileError
			require.ErrorAs(t, err, &file)
			assert.Equal(t, tc.hint, file.Hint())
			assert.Equal(t, tc.path(l), file.path)
		})
	}
}

// The probe lock of §5.6.1 fails the same way when its file cannot be opened,
// and a lock that could not be taken is not §5.6.2's timeout: nobody held it.
func TestAProbeLockThatCannotBeTakenIsAFileFailure(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, l.Init())
	path := l.ProbeLockFile("/src/acme/web", "laravel-pest")
	unopenable(t, path)

	held, err := l.LockProbe("/src/acme/web", "laravel-pest", time.Second)

	assert.Nil(t, held)
	var file *FileError
	require.ErrorAs(t, err, &file)
	assert.Equal(t, lockHint, file.Hint())
	assert.Equal(t, path, file.path)
	assert.ErrorIs(t, err, os.ErrPermission)
}

// §5.1.6's removal is a file failure too. A non-empty directory under the name
// makes os.Remove refuse while the lock, the directory and every read succeed.
func TestARemovalThatCannotLandIsAFileFailure(t *testing.T) {
	l := unlockedPR(t)
	name := FileSandboxBaseline
	require.NoError(t, os.MkdirAll(filepath.Join(l.PRDir("acme", "web", 42), name, "held"), dirPerm))
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	defer func() { assert.NoError(t, held.Unlock()) }()

	err = held.Remove(name)

	var file *FileError
	require.ErrorAs(t, err, &file)
	assert.Equal(t, removeHint, file.Hint())
	assert.ErrorIs(t, err, syscall.ENOTEMPTY)
}

// §2.2 keeps all of cr's state under one root. Audit round 1 measured
// LockPR("..", "..", 1) and a Write under outer/crhome creating outer/pr-1.lock
// and outer/pr-1/meta.json; every repository-keyed door now refuses a pair
// whose paths leave the state tree, before it creates or reads anything.
//
// What is asserted beside the refusal is the directory the root sits in: it
// stays empty, so nothing was created inside the root or outside it.
func TestAnOwnerAndRepositoryOutsideTheStateTreeAreRefused(t *testing.T) {
	doors := map[string]func(l Layout, owner, repo string) error{
		"LockPR": func(l Layout, owner, repo string) error {
			_, err := l.LockPR(owner, repo, 1)
			return err
		},
		"ReadPR": func(l Layout, owner, repo string) error {
			_, err := l.ReadPR(owner, repo, 1, FileMeta)
			return err
		},
		"EnsureRepo": func(l Layout, owner, repo string) error {
			return l.EnsureRepo(owner, repo)
		},
		"LockRepoWaivers": func(l Layout, owner, repo string) error {
			_, err := l.LockRepoWaivers(owner, repo)
			return err
		},
		"UpdateTriage": func(l Layout, owner, repo string) error {
			return UpdateTriage(l, owner, repo, func(held []map[string]any) []map[string]any { return held })
		},
		"UpdateRuleStats": func(l Layout, owner, repo string) error {
			return UpdateRuleStats(l, owner, repo, func(held []map[string]any) []map[string]any { return held })
		},
		"RemoveOrphanedSandbox": func(l Layout, owner, repo string) error {
			return l.RemoveOrphanedSandbox(owner, repo, 1)
		},
		"CopyIntoSandbox": func(l Layout, owner, repo string) error {
			_, err := l.CopyIntoSandbox(owner, repo, 1, os.TempDir(), ".")
			return err
		},
	}
	pairs := map[string][2]string{
		"both halves dot-dot":              {"..", ".."},
		"an owner of dot-dot":              {"..", "web"},
		"a repository of dot-dot":          {"acme", ".."},
		"an owner of dot and a dot-dot":    {".", ".."},
		"a repository climbing two levels": {"acme", "../.."},
	}
	for door, call := range doors {
		for pair, halves := range pairs {
			t.Run(door+" with "+pair, func(t *testing.T) {
				outer := t.TempDir()
				l := New(filepath.Join(outer, "crhome"))

				err := call(l, halves[0], halves[1])

				var outside *OutsideRootError
				require.ErrorAs(t, err, &outside)
				assert.Equal(t, l.StateDir(), outside.Under)
				assert.Equal(t, filepath.Join(l.StateDir(), halves[0], halves[1]), outside.Path)
				entries, err := os.ReadDir(outer)
				require.NoError(t, err)
				assert.Empty(t, entries, "a refused pair creates nothing, in the root or beside it")
			})
		}
	}
}

// The file name a held lock writes, removes or reads under is held to the pull
// request's own directory, so a name cannot climb into a sibling's state.
func TestAFileNameOutsideThePullRequestDirectoryIsRefused(t *testing.T) {
	l := unlockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	defer func() { assert.NoError(t, held.Unlock()) }()
	dir := l.PRDir("acme", "web", 42)
	sibling := filepath.Join("..", "pr-43", FileMeta)
	want := &OutsideRootError{Path: filepath.Join(dir, sibling), Under: dir}

	assert.Equal(t, want, held.Write(sibling, []byte("{}\n")))
	assert.Equal(t, want, held.Remove(sibling))
	_, err = l.ReadPR("acme", "web", 42, sibling)
	assert.Equal(t, want, err)
	assert.Equal(t, &OutsideRootError{Path: dir, Under: dir}, held.Write("", nil),
		"the directory itself is not a file of it")
	_, err = os.Stat(l.PRDir("acme", "web", 43))
	assert.ErrorIs(t, err, os.ErrNotExist)
}
