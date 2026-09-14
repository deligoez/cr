package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// RunnerLockFile is the lock a test runner cr starts in one pull request's
// sandbox inherits, and the record of the process group that runner leads.
//
// It sits beside PRLockFile under the locks directory §2.2 reserves for §5.6,
// and differs from it in one property: cr passes it to the runner as an open
// descriptor, so the lock stays held for as long as any process of that run is
// alive — including after cr itself was killed. That is what lets a later run
// tell a runner an earlier cr left behind from a process group number some
// unrelated program has since been given.
func (l Layout) RunnerLockFile(owner, repo string, pr int) string {
	return filepath.Join(l.LocksDir(), owner, repo, "pr-"+strconv.Itoa(pr)+".runner.lock")
}

// RunnerOwnerFile is the lock the cr running a runner holds itself and never
// passes on, so a held runner lock whose owner lock is free is a runner whose
// cr is gone.
func (l Layout) RunnerOwnerFile(owner, repo string, pr int) string {
	return filepath.Join(l.LocksDir(), owner, repo, "pr-"+strconv.Itoa(pr)+".runner-owner.lock")
}

// RunnerLock is held by a cr run for the life of one test runner it starts.
type RunnerLock struct {
	owner  *os.File
	runner *os.File
}

// LockRunner takes the two runner locks of one pull request before a runner is
// started in its sandbox.
//
// The owner lock is waited for: the only other holder is a later run briefly
// stopping a runner an earlier one left behind. The runner lock is only
// attempted. A process an earlier run started outside the runner's process
// group can hold it for as long as it lives, and refusing every run while it
// does would make the suite unrunnable for a reason no step of cr's clears.
func (l Layout) LockRunner(owner, repo string, pr int) (*RunnerLock, error) {
	if err := l.containRepo(owner, repo); err != nil {
		return nil, err
	}
	ownerPath := l.RunnerOwnerFile(owner, repo, pr)
	runnerPath := l.RunnerLockFile(owner, repo, pr)
	if err := makeDirs([]string{filepath.Dir(runnerPath)}); err != nil {
		return nil, err
	}
	held, err := lockedFile(ownerPath, syscall.LOCK_EX)
	if err != nil {
		return nil, err
	}
	runner, err := lockedFile(runnerPath, syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		runner, err = openLockFile(runnerPath)
	}
	if err != nil {
		return nil, errors.Join(err, held.Close())
	}
	return &RunnerLock{owner: held, runner: runner}, nil
}

// Inherited is the descriptor the runner is started with, which carries the
// runner lock into every process of its group.
func (k *RunnerLock) Inherited() *os.File { return k.runner }

// Started records the process group the runner leads.
func (k *RunnerLock) Started(group int) error {
	if err := k.runner.Truncate(0); err != nil {
		return FileFailure("write", k.runner.Name(), lockHint, err)
	}
	if _, err := k.runner.WriteAt([]byte(strconv.Itoa(group)), 0); err != nil {
		return FileFailure("write", k.runner.Name(), lockHint, err)
	}
	return nil
}

// Release clears the recorded group and closes both locks.
//
// The runner lock is closed rather than unlocked. It is one lock shared with
// every process the runner started, and an unlock would release it for a
// process still alive; a close releases only cr's own hold on it.
func (k *RunnerLock) Release() error {
	var failures []error
	if err := k.runner.Truncate(0); err != nil {
		failures = append(failures, FileFailure("write", k.runner.Name(), lockHint, err))
	}
	if err := k.runner.Close(); err != nil {
		failures = append(failures, FileFailure("release", k.runner.Name(), lockHint, err))
	}
	if err := k.owner.Close(); err != nil {
		failures = append(failures, FileFailure("release", k.owner.Name(), lockHint, err))
	}
	return errors.Join(failures...)
}

// LeftRunner is a runner an earlier cr run started in a pull request's sandbox
// and did not outlive: its runner lock is still held, its owner lock is free,
// and it recorded the process group it leads.
type LeftRunner struct {
	// Group is the process group the runner leads.
	Group int
	owner *os.File
	path  string
}

// LeftRunner reports the runner an earlier cr run left alive in one pull
// request's sandbox, and nil when there is none.
//
// A held runner lock alone is not enough. A live cr holds it while its own
// runner runs, and that cr's owner lock says so; a process an earlier run
// started outside the runner's group can hold it after a run that finished
// normally, and that run cleared the record. The returned value holds the
// owner lock, so no run starts a runner while the one left behind is stopped.
func (l Layout) LeftRunner(owner, repo string, pr int) (*LeftRunner, error) {
	if err := l.containRepo(owner, repo); err != nil {
		return nil, err
	}
	path := l.RunnerLockFile(owner, repo, pr)
	free, err := runnerLockFree(path)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && free) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	held, err := lockedFile(l.RunnerOwnerFile(owner, repo, pr), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Join(FileFailure("read", path, lockHint, err), held.Close())
	}
	// A group of 1 or less names no runner: kill(2) reads -1 as every
	// process cr may signal.
	group, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil || group <= 1 {
		return nil, held.Close()
	}
	return &LeftRunner{Group: group, owner: held, path: path}, nil
}

// Gone reports whether every process holding the runner lock has exited.
func (r *LeftRunner) Gone() (bool, error) {
	return runnerLockFree(r.path)
}

// Release clears the record of the runner that was stopped and gives up the
// owner lock.
func (r *LeftRunner) Release() error {
	var failures []error
	if err := os.Truncate(r.path, 0); err != nil {
		failures = append(failures, FileFailure("write", r.path, lockHint, err))
	}
	if err := r.owner.Close(); err != nil {
		failures = append(failures, FileFailure("release", r.owner.Name(), lockHint, err))
	}
	return errors.Join(failures...)
}

// runnerLockFree reports whether no process holds the runner lock at path.
func runnerLockFree(path string) (bool, error) {
	probe, err := os.OpenFile(path, os.O_RDWR, filePerm)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
		return false, FileFailure("open", path, lockHint, err)
	}
	defer probe.Close()
	switch err := syscall.Flock(int(probe.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.EWOULDBLOCK):
		return false, nil
	default:
		return false, FileFailure("lock", path, lockHint, err)
	}
}

// openLockFile opens a lock file for reading and writing, creating it.
func openLockFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, filePerm)
	if err != nil {
		return nil, FileFailure("open", path, lockHint, err)
	}
	return file, nil
}

// lockedFile opens a lock file and takes how on it. A lock that would block
// under LOCK_NB is returned as the bare syscall.EWOULDBLOCK, so the caller can
// tell a held lock from a failure.
func lockedFile(path string, how int) (*os.File, error) {
	file, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), how); err != nil {
		closed := file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, syscall.EWOULDBLOCK
		}
		return nil, errors.Join(FileFailure("lock", path, lockHint, fmt.Errorf("flock: %w", err)), closed)
	}
	return file, nil
}
