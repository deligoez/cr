package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// ProfilesLockFile is the advisory lock over writes to the profiles directory.
//
// It sits directly under §2.2's locks/, where PRLockFile spends
// locks/<owner>/ on pull-request locks. A GitHub owner name holds no dot, so no
// owner's directory can be named profiles.lock.
func (l Layout) ProfilesLockFile() string { return filepath.Join(l.LocksDir(), "profiles.lock") }

// ProfileRefresh is what RefreshProfile did to one profile file.
type ProfileRefresh int

const (
	// ProfileKept is a file that existed and was left as it was.
	ProfileKept ProfileRefresh = iota
	// ProfileCreated is a file that did not exist and was written.
	ProfileCreated
	// ProfileReplaced is a file that existed and was published over.
	ProfileReplaced
)

// profileReadHint is §12.4's next actionable step for a profile file `cr init`
// could not read before deciding whether to replace it.
const profileReadHint = "cr init reads each shipped profile's file under ~/.cr/profiles before deciding " +
	"whether it may update it; check that the path the message names is a readable regular file"

// RefreshProfile writes one shipped profile into the profiles directory.
//
// An absent file is created with content, as EnsureProfile creates it. An
// existing file is handed to replace, and published over with content only
// when replace says so; otherwise it is left exactly as it was. The read, the
// decision and the write happen under one exclusive lock, so two runs cannot
// both decide on bytes one of them has already replaced, and the write
// publishes by rename, so a command loading the profile without the lock reads
// the previous file or the new one whole.
func (l Layout) RefreshProfile(id, content string, replace func(onDisk []byte) bool) (ProfileRefresh, error) {
	lock := l.ProfilesLockFile()
	if err := makeDirs([]string{l.LocksDir(), l.ProfilesDir()}); err != nil {
		return ProfileKept, err
	}
	held := flock.New(lock)
	if err := held.Lock(); err != nil {
		return ProfileKept, FileFailure("lock", lock, lockHint, err)
	}
	outcome, err := l.refreshProfile(id, content, replace)
	if uerr := held.Unlock(); uerr != nil {
		return outcome, errors.Join(err, FileFailure("release", lock, lockHint, uerr))
	}
	return outcome, err
}

// refreshProfile is RefreshProfile's work, done while the lock is held.
func (l Layout) refreshProfile(id, content string, replace func(onDisk []byte) bool) (ProfileRefresh, error) {
	path := l.Profile(id)
	onDisk, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := l.EnsureProfile(id, content); err != nil {
			return ProfileKept, err
		}
		return ProfileCreated, nil
	case err != nil:
		return ProfileKept, FileFailure("read", path, profileReadHint, err)
	case !replace(onDisk):
		return ProfileKept, nil
	}
	if err := writeAtomic(path, []byte(content)); err != nil {
		return ProfileKept, err
	}
	return ProfileReplaced, nil
}
