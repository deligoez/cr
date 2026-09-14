package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/deligoez/cr/internal/text"
)

// DirProbeLocks holds the §5.6 probe locks inside the locks directory. It is a
// directory of its own so the probe lock's tree — which mirrors the path of the
// repository under review — can never collide with the owner directories
// PRLockFile nests under.
const DirProbeLocks = "probe"

// probeLockRetry is how often a waiter re-attempts the lock. §5.6.2 bounds the
// wait and says nothing about how it is spent; a tenth of a second is short
// enough that a suite finishing is followed promptly and long enough that the
// wait costs nothing to hold.
const probeLockRetry = 100 * time.Millisecond

// ProbeLockFile is the advisory lock guarding probe and test runs (§5.6.1).
//
// The name is the absolute path of the repository under review together with
// the profile id, and both halves are load-bearing. Keying on the pull request
// would let two pull requests of one repository run the suite at the same time
// against the one test database the profile configures, which is the collision
// §5.6.1 exists to prevent; keying on the repository alone would make two
// unrelated checkouts wait on each other for nothing.
//
// The path is carried as a tree rather than digested into a name. §1.4 defines
// cr's one hash and internal/text fences the ingredient, so there is no second
// digest to reach for — and there is no need for one: the repository path is
// already a sequence of legal path components, and laying it out as directories
// keeps the lock file legible to whoever is wondering what is holding it. The
// profile id is the last component, so a repository at `/a/b` and one at
// `/a/b/c` land at `.../a/b/<id>.lock` and `.../a/b/c/<id>.lock` and never at
// one name.
func (l Layout) ProbeLockFile(repoPath, profileID string) string {
	return filepath.Join(l.LocksDir(), DirProbeLocks, repoPath, profileID+".lock")
}

// ProbeLockedError reports §5.6.2's timeout: the lock was held for the whole of
// `probe.lock_timeout_seconds` and this run never got it.
//
// §11.2 codes it 4. The command line is right and every file named was read;
// what refuses is that another cr run is using the same repository and profile,
// which no retyping changes.
type ProbeLockedError struct {
	// RepoPath and ProfileID are the two halves of the lock's name, so
	// the reader is told which pair is busy rather than which file is.
	RepoPath  string
	ProfileID string
	// Waited is how long the run waited before giving up, which is the
	// configured `probe.lock_timeout_seconds`.
	Waited time.Duration
}

// ProbeLockedHint is §12.4's next actionable step for a ProbeLockedError, and
// the one step: internal/cli's exit code table prints it beside the message,
// and the message does not spell a step of its own. Audit round 1 found the
// two disagreeing — the message said to raise the setting "with `cr config`",
// which only prints the configuration, while the table said to wait.
const ProbeLockedHint = "wait for the other cr run on this repository and profile to finish, " +
	"then run the command again"

func (e *ProbeLockedError) Error() string {
	return fmt.Sprintf(
		"another cr run holds the probe lock for %s under profile %s: waited %s, "+
			"which is probe.lock_timeout_seconds",
		e.RepoPath, e.ProfileID, e.Waited,
	)
}

// ProbeLockOutsideError reports a probe lock whose file would resolve outside
// §5.6.1's probe locks directory, and carries the OutsideRootError the other
// doors of §2.2's tree refuse such a path with.
//
// It is a type of its own for the step it takes. Those doors are keyed by an
// owner and a repository, which `--repo` names, so their refusal sends the
// reader to the flag. This one is keyed by the checkout's absolute path, which
// filepath.Abs has already cleaned, and by the profile id meta.json records,
// and `--repo` answers neither.
type ProbeLockOutsideError struct {
	// RepoPath and ProfileID are the two halves of the lock's name.
	RepoPath  string
	ProfileID string
	// Outside is the refusal itself: the path the halves led to and the
	// directory it had to stay inside.
	Outside *OutsideRootError
}

func (e *ProbeLockOutsideError) Error() string {
	return fmt.Sprintf("the probe lock for %s under profile %q: %v",
		e.RepoPath, e.ProfileID, e.Outside)
}

// Unwrap exposes the OutsideRootError, so a caller asking whether a path left
// the state tree is told that this one did.
func (e *ProbeLockOutsideError) Unwrap() error { return e.Outside }

// ProbeLock is the advisory lock of §5.6.1, held for the length of one probe or
// test run.
//
// It is a separate type from Lock, and deliberately so. Lock guards one pull
// request's state files and Write is a method on it, because §2.3.1 makes
// holding it the only way to reach them; this one guards a resource cr does not
// own at all — the test database, the ports, whatever the suite touches — so it
// offers nothing but the holding.
//
// It is two flocks over one name. home lives under the state root and clone
// under os.TempDir: the test database is shared by every run against the
// checkout, whichever `CR_HOME` each run reads its state from, and a lock under
// the state root alone let two runs with different state roots run the suite at
// once.
type ProbeLock struct {
	home      *flock.Flock
	clone     *flock.Flock
	repoPath  string
	profileID string
}

// cloneLockPrefix begins the name of every §5.6.1 lock under os.TempDir, so a
// person listing the temporary directory can tell what created the file.
const cloneLockPrefix = "cr-probe-"

// cloneLockFile is the file of §5.6.1's second lock: the one every cr run that
// resolves the same os.TempDir takes for the checkout and profile, whatever
// state root it runs under. That directory is the per-user $TMPDIR on macOS and
// /tmp on Linux, so the lock is shared per user on the one and per machine on
// the other.
//
// The name is a digest rather than the path laid out as a tree, as the lock
// under the state root is. A digest is a name no repository path or profile id
// can lead out of the temporary directory, which a `..` in either half would do
// to a joined path, and its length does not grow with the checkout's path, so
// it never meets the limit a file name has. It also creates no directory in a
// directory other users share.
//
// Each half is rendered as hex before the digest, with a slash between: hex is
// ASCII and holds no whitespace, so §1.4's normalisation leaves the input as it
// is, and hex holds no slash, so two different pairs never render to one input.
// The repository path is cleaned first, as filepath.Join cleans it for the lock
// under the state root, so both locks treat `/a/../a/b` and `/a/b` as one
// checkout.
func cloneLockFile(repoPath, profileID string) (string, error) {
	key, err := text.NormalisedHash(fmt.Sprintf("%x/%x", filepath.Clean(repoPath), profileID))
	if err != nil {
		return "", err
	}
	return filepath.Join(os.TempDir(), cloneLockPrefix+key+".lock"), nil
}

// LockProbe takes §5.6.1's advisory lock, waiting up to timeout for it.
//
// The lock is two flocks, taken in order: the one under the state root first,
// then the one under os.TempDir that runs with other state roots take too. Both
// are waited on within the one timeout, so §5.6.2's wait is never longer than
// `probe.lock_timeout_seconds` however the two are held. Every run takes them in
// the same order, so no two runs can each hold one while waiting on the other.
//
// Neither file is created beside the repository it names: §2.2 permits cr no
// write inside the repository under review but §5.1.1's worktree registration,
// and a lock file there would be a second one.
//
// A timeout that expires on either fails with ProbeLockedError, which §11.2
// codes 4, and releases the first if it was the second that could not be taken.
// It is a refusal rather than a run that proceeds anyway, because the whole point
// of the lock is that the second run would measure a suite the first one is
// already inside.
//
// A repository path or profile id whose lock file would leave the probe locks
// directory is refused with ProbeLockOutsideError before anything is created,
// as every repository-keyed door of §2.2's tree refuses a path that leaves it.
func (l Layout) LockProbe(repoPath, profileID string, timeout time.Duration) (*ProbeLock, error) {
	path := l.ProbeLockFile(repoPath, profileID)
	under := filepath.Join(l.LocksDir(), DirProbeLocks)
	if contain(under, path) != nil {
		return nil, &ProbeLockOutsideError{RepoPath: repoPath, ProfileID: profileID,
			Outside: &OutsideRootError{Path: path, Under: under}}
	}
	clonePath, err := cloneLockFile(repoPath, profileID)
	if err != nil {
		return nil, err
	}
	if err := makeDirs([]string{filepath.Dir(path)}); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	refused := &ProbeLockedError{RepoPath: repoPath, ProfileID: profileID, Waited: timeout}
	home, err := takeProbeLock(ctx, path, refused)
	if err != nil {
		return nil, err
	}
	clone, err := takeProbeLock(ctx, clonePath, refused)
	if err != nil {
		return nil, errors.Join(err, releaseProbeLock(home))
	}
	return &ProbeLock{home: home, clone: clone, repoPath: repoPath, profileID: profileID}, nil
}

// takeProbeLock waits on one of §5.6.1's two flocks until ctx is done, and
// answers a wait that ran out with refused.
func takeProbeLock(ctx context.Context, path string, refused *ProbeLockedError) (*flock.Flock, error) {
	held := flock.New(path)
	taken, err := held.TryLockContext(ctx, probeLockRetry)
	switch {
	case taken:
		return held, nil
	case err == nil || errors.Is(err, context.DeadlineExceeded):
		return nil, refused
	}
	return nil, FileFailure("lock", path, lockHint, err)
}

// releaseProbeLock releases one of §5.6.1's two flocks and leaves its file.
func releaseProbeLock(held *flock.Flock) error {
	if err := held.Unlock(); err != nil {
		return FileFailure("release", held.Path(), lockHint, err)
	}
	return nil
}

// CollisionWarning is §5.6.3's warning, and it is a warning rather than an
// enforcement because there is nothing here to enforce.
//
// The lock covers cr's own runs and can cover nothing else: a `pest` or a `go
// test` the developer started in another terminal takes no lock of cr's, holds
// the same database, and cr cannot see it. §5.6.3 asks cr to say so instead of
// letting a held lock read as a guarantee — a run that failed for a reason the
// lock never covered is exactly the run someone would otherwise attribute to
// the code under review.
//
// It is derived from the lock's own two halves, so what the reader is told and
// what the lock actually guards cannot drift apart.
func (k *ProbeLock) CollisionWarning() string {
	return "the probe lock covers cr's own runs only, per §5.6.3: a test run you start yourself " +
		"in " + k.repoPath + " can still collide with this one"
}

// Unlock releases the lock and leaves both files on disk, for the reason
// Lock.Unlock does: an advisory lock binds to an inode, so unlinking the file
// would let the next waiter open the same path, receive a fresh inode, and run
// alongside the holder while every call kept reporting success.
//
// The two flocks are released in the reverse of the order LockProbe took them,
// and both are released even when the first release fails.
func (k *ProbeLock) Unlock() error {
	return errors.Join(releaseProbeLock(k.clone), releaseProbeLock(k.home))
}
