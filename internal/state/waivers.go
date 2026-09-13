package state

import (
	"path/filepath"

	"github.com/gofrs/flock"
)

// waiverLocksDir holds the advisory locks over the repository-wide waiver
// stores of §7.4.4.
//
// It is a directory of its own inside §2.2's locks/ for a reason stronger than
// the one contextLocksDir gives. PRLockFile already spends locks/<owner>/<repo>
// as a directory of pull-request locks, so a lock file named
// locks/<owner>/<repo>.lock could not be created beside it at all.
const waiverLocksDir = "waivers"

// RepoWaiverLockFile is the advisory lock guarding one repository's waiver
// store (§7.4.4).
func (l Layout) RepoWaiverLockFile(owner, repo string) string {
	return filepath.Join(l.LocksDir(), waiverLocksDir, owner, repo+".lock")
}

// WaiverLock is the exclusive advisory lock over one repository's waiver store
// — §2.2's waivers/<owner>/<repo>.ndjson — and the only route to that file's
// records.
//
// §2.3.1 does not reach this file, and that is the argument for the lock rather
// than against it. §2.3.1 locks writes to *per-PR* state; this file is §2.2
// state shared by every pull request of one repository, so two `cr draft` runs
// on two pull requests of that repository take two different §2.3.1 locks, see
// the same waivers, and each publish the whole file. With no lock of its own
// the second publication drops the first's waiver, the finding it silenced
// comes back, and nothing on disk says why.
//
// The lock covers more than the write, for the reason ContextLock's does:
// §7.4.7 gives a waiver an id, and the id is allocated by counting over the
// waivers the file already holds, which makes the read, the allocation and the
// write one critical section. That is why WaiverRecords is a method on the held
// lock — a caller cannot count the waivers without holding the lock it will
// write under.
//
// The write still publishes by rename. §2.3.2's reason applies to this file
// even though §2.3 does not: §7.4.7's listing reads it without taking the lock,
// so a reader arriving mid-write must see the whole previous file rather than
// half of the new one.
type WaiverLock struct {
	held *flock.Flock
	path string
}

// LockRepoWaivers blocks until it holds the exclusive advisory lock over one
// repository's waiver store. Like LockContext it creates the directory it will
// write in, so a writer never depends on EnsureRepo having run first.
func (l Layout) LockRepoWaivers(owner, repo string) (*WaiverLock, error) {
	if err := l.containRepo(owner, repo); err != nil {
		return nil, err
	}
	lock := l.RepoWaiverLockFile(owner, repo)
	store := l.WaiversFile(owner, repo)
	if err := makeDirs([]string{filepath.Dir(lock), filepath.Dir(store)}); err != nil {
		return nil, err
	}
	held := flock.New(lock)
	if err := held.Lock(); err != nil {
		return nil, FileFailure("lock", lock, lockHint, err)
	}
	return &WaiverLock{held: held, path: store}, nil
}

// Unlock releases the lock and leaves its file on disk, for the reason
// Lock.Unlock does: an advisory lock binds to an inode and not to a path.
func (k *WaiverLock) Unlock() error {
	if err := k.held.Unlock(); err != nil {
		return FileFailure("release", k.held.Path(), lockHint, err)
	}
	return nil
}

// WaiverRecords decodes the locked repository's waiver store.
func WaiverRecords[T any](k *WaiverLock) ([]T, error) {
	return storeRecords[T](k.path)
}

// ReadWaiverRecords decodes one repository's waiver store without taking the
// lock, per §2.3.2. It is the read §7.4.7's listing makes, and the one §6.4.4
// drops a finding against.
//
// A lock-free read is safe here for the reason ReadContextRecords gives:
// WriteWaiverRecords publishes by rename, so a reader arriving mid-append sees
// the whole of one version rather than half of two. Taking the lock to read
// would also make a read-only command create directories inside ~/.cr, since
// LockRepoWaivers creates the ones it will write in.
func ReadWaiverRecords[T any](l Layout, owner, repo string) ([]T, error) {
	return storeRecords[T](l.WaiversFile(owner, repo))
}

// WriteWaiverRecords publishes the locked repository's waiver store, replacing
// it with records.
func WriteWaiverRecords[T any](k *WaiverLock, records []T) error {
	body, err := encodeRecords(k.path, records)
	if err != nil {
		return err
	}
	return writeAtomic(k.path, body)
}
