package state

import (
	"errors"
	"io/fs"
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
// depends on EnsurePR having run first. Neither is created for an owner and
// repository whose paths would leave the state tree.
func (l Layout) LockPR(owner, repo string, pr int) (*Lock, error) {
	if err := l.containRepo(owner, repo); err != nil {
		return nil, err
	}
	path := l.PRLockFile(owner, repo, pr)
	dir := l.PRDir(owner, repo, pr)
	if err := makeDirs([]string{filepath.Dir(path), dir}); err != nil {
		return nil, err
	}
	held := flock.New(path)
	if err := held.Lock(); err != nil {
		return nil, FileFailure("lock", path, lockHint, err)
	}
	return &Lock{held: held, dir: dir}, nil
}

// lockHint is §12.4's next actionable step for an advisory lock of §2.3.1 or
// §5.6.1 that could not be taken or released.
//
// It is a file failure and §11.2 codes it 3, as writeHint's is: the command
// line was right, and what refused was the lock file under the state root.
const lockHint = "cr locks through a file under the state root's locks directory; " +
	"check that the path the message names is a regular file cr can open for writing"

// Write publishes one file of the locked pull request's state.
//
// The name is then checked against §2.3's table, so the table is what decides
// which files a pull request's state directory holds rather than which call
// sites somebody remembered. Remove is deliberately not fenced the same way:
// it creates nothing, and a removal of a name the table does not give is
// already a removal of a file that is not there.
//
// Containment is judged first, and the order is the message rather than the
// outcome — both refuse. A name climbing into a sibling pull request's state
// is answered with the error that says so, instead of being reported as a file
// §2.3 does not name, which would send the reader to the wrong table.
func (k *Lock) Write(name string, data []byte) error {
	path := filepath.Join(k.dir, name)
	if err := contain(k.dir, path); err != nil {
		return err
	}
	if err := checkPRFile(name); err != nil {
		return err
	}
	return writeAtomic(path, data)
}

// removeHint is §12.4's next actionable step for a file of §2.3 that could not
// be removed: a removal, like §2.3's rename, is a change to the directory.
const removeHint = "removing a file changes the pull request's state directory, " +
	"so the directory itself has to be writable; check its permissions"

// Remove deletes one file of the locked pull request's state, and reports a
// file that was not there as success.
//
// It exists for §5.1.6's baseline, which is invalidated rather than rewritten
// when the sandbox it describes is recreated: the new sandbox's baseline is
// not known until §5.1.3's setup has finished, and between the two moments the
// honest answer is that there is none. An absent file is success because that
// is the state the caller asked for, and a first creation would otherwise have
// to know whether it was a first.
func (k *Lock) Remove(name string) error {
	path := filepath.Join(k.dir, name)
	if err := contain(k.dir, path); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return FileFailure("remove", path, removeHint, err)
	}
	return nil
}

// Unlock releases the lock and deliberately leaves its file on disk.
//
// The leftover file is the mechanism, not litter. An advisory lock binds to an
// inode, not to a path: unlinking the file on release would let the next waiter
// open the same path, receive a fresh inode, and run alongside the holder,
// while every call kept reporting success. Never "clean up" the lock file.
func (k *Lock) Unlock() error {
	if err := k.held.Unlock(); err != nil {
		return FileFailure("release", k.held.Path(), lockHint, err)
	}
	return nil
}

// ReadPR reads one file of a pull request's state. It takes no lock, because
// §2.3.2 requires reads to be lock-free, which is what obliges every write to
// publish by rename.
func (l Layout) ReadPR(owner, repo string, pr int, name string) ([]byte, error) {
	if err := l.containRepo(owner, repo); err != nil {
		return nil, err
	}
	path := l.PRFile(owner, repo, pr, name)
	if err := contain(l.PRDir(owner, repo, pr), path); err != nil {
		return nil, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		// A file of §2.2's tree the command required, so §11.2 codes it
		// 3 and the hint names what writes it. It was a bare
		// fmt.Errorf until unreadable-input-exit-code, and exitCodeFor
		// has no mapping for one: measured, `cr record` on a round
		// missing probes.ndjson exited 2.
		return nil, FileFailure("read", path, readHint(name), err)
	}
	return body, nil
}

// writeHint is §12.4's next actionable step for a §2.3 write that did not land.
//
// Every one of them is a file failure and §11.2 codes it 3. It is not a usage
// error: measured 2026-09-12 by the mutation slice, `cr record` against a
// read-only pull-request directory failed inside writeAtomic with a bare
// `cannot create a temporary file in <dir>` and exited 2, telling the user to
// retype a command line that was right.
const writeHint = "§2.3 publishes every write by renaming a temporary file into place, " +
	"so the directory itself has to be writable; check its permissions and free space"

// writeAtomic publishes data at path by writing a temporary file beside it and
// renaming it into place.
//
// The lock alone does not make a write safe. Readers hold no lock at all
// (§2.3.2), so one can arrive in the middle of any write, and an in-place write
// would hand it half a document. A rename within a single directory is atomic
// on every platform cr targets, so the reader sees either the previous file or
// the whole new one. os.CreateTemp creates the temporary file with filePerm.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return FileFailure("create a temporary file beside", path, writeHint, err)
	}
	// A no-op once the rename has succeeded; it clears the temporary file on
	// every path that fails before it.
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return FileFailure("write", tmp.Name(), writeHint, err)
	}
	if err := tmp.Close(); err != nil {
		return FileFailure("close", tmp.Name(), writeHint, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return FileFailure("publish", path, writeHint, err)
	}
	return nil
}
