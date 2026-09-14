package git

import (
	"errors"
	"fmt"
)

// MissingCommitError reports a commit the repository under review does not
// hold: a head GitHub reports that was never fetched into the clone, or one a
// force-push made unreachable and the clone never received.
//
// It is its own type rather than the CommandError the next read of that commit
// would fail with, because the two call for different next steps. A git that
// refused for a reason of its own is a command to run by hand and read; a
// commit that is simply absent is a fetch.
type MissingCommitError struct {
	// Dir is the repository that was asked.
	Dir string
	// Commit is the revision it does not hold.
	Commit string
}

func (e *MissingCommitError) Error() string {
	return fmt.Sprintf("the repository at %s does not hold commit %s", e.Dir, e.Commit)
}

// exitCoder is the part of os/exec's ExitError this file reads. It is named
// here rather than imported, because run.go is the one file of this package
// that may start a process, and TestCrReachesTheNetworkThroughOneRunnerAndNoOtherWay
// holds every other file to not importing os/exec.
type exitCoder interface {
	ExitCode() int
}

// RequireCommit answers nil when dir holds commit as a commit object, and a
// *MissingCommitError when it does not.
//
// `rev-parse --verify --quiet` exits 1 and prints nothing for a revision it
// cannot resolve, and 128 with a message when it cannot run at all — outside a
// repository, for one. Only the first is an absent commit; the second stays the
// CommandError it is.
func RequireCommit(dir, commit string) error {
	_, err := run(dir, "rev-parse", "--verify", "--quiet", "--end-of-options", commit+"^{commit}")
	var failed *CommandError
	if !errors.As(err, &failed) || failed.Stderr != "" {
		return err
	}
	var exited exitCoder
	if errors.As(failed.Err, &exited) && exited.ExitCode() == 1 {
		return &MissingCommitError{Dir: dir, Commit: commit}
	}
	return err
}
