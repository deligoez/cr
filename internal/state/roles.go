package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// RolesLockFile is the advisory lock over writes to the roles directory.
//
// It sits directly under §2.2's locks/, beside ProfilesLockFile and for the
// same reason: a GitHub owner name holds no dot, so no owner's directory can be
// named roles.lock.
func (l Layout) RolesLockFile() string { return filepath.Join(l.LocksDir(), "roles.lock") }

// RoleRefresh is what RefreshRole did to one role file.
type RoleRefresh int

const (
	// RoleAbsent is a file that was not there and was not written,
	// because §2.5.2 gives writing an absent role file to
	// `cr init --eject-roles` alone.
	RoleAbsent RoleRefresh = iota
	// RoleKept is a file that existed and was left as it was.
	RoleKept
	// RoleCreated is a file that did not exist and was ejected.
	RoleCreated
	// RoleReplaced is a file that existed and was published over.
	RoleReplaced
)

// roleReadHint is §12.4's next actionable step for a role file `cr init` could
// not read before deciding whether to replace it.
const roleReadHint = "cr init reads each ejected role's file under ~/.cr/roles before deciding " +
	"whether it may update it; check that the path the message names is a readable regular file"

// RefreshRole writes one shipped role into the roles directory.
//
// §2.5.2 splits the two writes and this carries both. An absent file is
// ejected with content only when eject says so, because §2.5.2 gives writing
// one to `cr init --eject-roles` and a plain `cr init` must not hand a user a
// roles directory they never asked to own — §2.5.4 resolves the built-in layer
// for them already. An existing file is handed to replace, and published over
// with content only when replace says so; otherwise it is left exactly as it
// was, which is what keeps an edit safe.
//
// The read, the decision and the write happen under one exclusive lock, so two
// runs cannot both decide on bytes one of them has already replaced, and the
// write publishes by rename, so a command loading the role without the lock
// reads the previous file or the new one whole.
func (l Layout) RefreshRole(id, content string, eject bool, replace func(onDisk []byte) bool) (RoleRefresh, error) {
	lock := l.RolesLockFile()
	if err := makeDirs([]string{l.LocksDir(), l.RolesDir()}); err != nil {
		return RoleAbsent, err
	}
	held := flock.New(lock)
	if err := held.Lock(); err != nil {
		return RoleAbsent, FileFailure("lock", lock, lockHint, err)
	}
	outcome, err := l.refreshRole(id, content, eject, replace)
	if uerr := held.Unlock(); uerr != nil {
		return outcome, errors.Join(err, FileFailure("release", lock, lockHint, uerr))
	}
	return outcome, err
}

// refreshRole is RefreshRole's work, done while the lock is held.
func (l Layout) refreshRole(id, content string, eject bool, replace func(onDisk []byte) bool) (RoleRefresh, error) {
	path := l.Role(id)
	onDisk, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist) && !eject:
		return RoleAbsent, nil
	case errors.Is(err, fs.ErrNotExist):
		if err := l.EnsureRole(id, content); err != nil {
			return RoleAbsent, err
		}
		return RoleCreated, nil
	case err != nil:
		return RoleAbsent, FileFailure("read", path, roleReadHint, err)
	case !replace(onDisk):
		return RoleKept, nil
	}
	if err := writeAtomic(path, []byte(content)); err != nil {
		return RoleKept, err
	}
	return RoleReplaced, nil
}
