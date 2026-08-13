package state

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// Lock is the exclusive advisory lock over one pull request's state (§2.3.1).
//
// It is also the only route to that state's files: Write is a method on the
// held lock rather than on the layout, so a writer cannot reach the files of
// §2.3 without having taken the lock first.
type Lock struct {
	held *flock.Flock
	dir  string
}

// LockPR blocks until it holds the exclusive advisory lock over one pull
// request's state. §2.3.1 puts no deadline on the wait; the bounded one of
// §5.6 belongs to the probe lock, which guards different work under a different
// name.
//
// The state directory is created alongside the lock file so that a writer never
// depends on EnsurePR having run first.
func (l Layout) LockPR(owner, repo string, pr int) (*Lock, error) {
	path := l.PRLockFile(owner, repo, pr)
	dir := l.PRDir(owner, repo, pr)
	if err := makeDirs([]string{filepath.Dir(path), dir}); err != nil {
		return nil, err
	}
	held := flock.New(path)
	if err := held.Lock(); err != nil {
		return nil, fmt.Errorf("cannot lock %s: %w", path, err)
	}
	return &Lock{held: held, dir: dir}, nil
}

// Write publishes one file of the locked pull request's state.
func (k *Lock) Write(name string, data []byte) error {
	return writeAtomic(filepath.Join(k.dir, name), data)
}

// Unlock releases the lock and deliberately leaves its file on disk.
//
// The leftover file is the mechanism, not litter. An advisory lock binds to an
// inode, not to a path: unlinking the file on release would let the next waiter
// open the same path, receive a fresh inode, and run alongside the holder,
// while every call kept reporting success. Never "clean up" the lock file.
func (k *Lock) Unlock() error {
	if err := k.held.Unlock(); err != nil {
		return fmt.Errorf("cannot release %s: %w", k.held.Path(), err)
	}
	return nil
}

// ReadPR reads one file of a pull request's state. It takes no lock, because
// §2.3.2 requires reads to be lock-free, which is what obliges every write to
// publish by rename.
func (l Layout) ReadPR(owner, repo string, pr int, name string) ([]byte, error) {
	path := filepath.Join(l.PRDir(owner, repo, pr), name)
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	return body, nil
}

// writeAtomic publishes data at path by writing a temporary file beside it and
// renaming it into place.
//
// The lock alone does not make a write safe. Readers hold no lock at all
// (§2.3.2), so one can arrive in the middle of any write, and an in-place write
// would hand it half a document. A rename within a single directory is atomic
// on every platform cr targets, so the reader sees either the previous file or
// the whole new one. os.CreateTemp creates the temporary file with filePerm.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("cannot create a temporary file in %s: %w", dir, err)
	}
	// A no-op once the rename has succeeded; it clears the temporary file on
	// every path that fails before it.
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("cannot write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cannot close %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("cannot publish %s: %w", path, err)
	}
	return nil
}
