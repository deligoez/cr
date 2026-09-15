package state

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"syscall"
)

// The descriptors a held runner is started with. The runner lock is at 3,
// where an unheld runner has always had it, and the two pipes of the hold
// follow it.
const (
	heldReleaseFD = 4
	heldFailureFD = 5
)

// RunnerHold is the pair of pipes that keeps a test runner from starting until
// the cr that started it has recorded the runner's process group.
//
// The process cr starts is its own binary, not the runner. That process reads
// the release pipe and replaces itself with the runner only once a byte
// arrives, which cr writes after RunnerLock.Started. A cr killed before then
// never writes it, the kernel closes its end, and the read ends in EOF: the
// held process exits having run nothing. So no moment exists at which a runner
// is alive and its group is not in the runner lock for a later run to stop.
//
// The failure pipe carries back what exec reported when it could not replace
// the process, the answer cmd.Start gave before there was a hold. The held
// process marks its end close-on-exec, so a runner that did start closes it
// and cr reads EOF.
type RunnerHold struct {
	release   *os.File
	failure   *os.File
	inherited []*os.File
}

// Hold opens the pipes one held runner of this lock is started with.
func (k *RunnerLock) Hold() (*RunnerHold, error) {
	releaseRead, releaseWrite, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("cannot open the pipe that holds the test runner: %w", err)
	}
	failureRead, failureWrite, err := os.Pipe()
	if err != nil {
		_ = releaseRead.Close()
		_ = releaseWrite.Close()
		return nil, fmt.Errorf("cannot open the pipe that holds the test runner: %w", err)
	}
	return &RunnerHold{
		release:   releaseWrite,
		failure:   failureRead,
		inherited: []*os.File{k.Inherited(), releaseRead, failureWrite},
	}, nil
}

// Inherited is the descriptors the held process is started with, from 3 on:
// the runner lock, the read end of the release pipe and the write end of the
// failure pipe.
func (h *RunnerHold) Inherited() []*os.File { return h.inherited }

// Started closes cr's copies of the ends the held process was given. The
// failure pipe reaches EOF only once every copy of its write end is closed.
func (h *RunnerHold) Started() {
	for _, end := range h.inherited[1:] {
		_ = end.Close()
	}
}

// Abandon closes every end cr holds without releasing, so a held process that
// was started exits having run nothing.
func (h *RunnerHold) Abandon() {
	h.Started()
	_ = h.release.Close()
	_ = h.failure.Close()
}

// Release lets the held process become the runner at path, and returns what
// exec reported when it could not, in the shape os.StartProcess gives the same
// failure; nil means the runner is running.
//
// The write's error is not returned. A held process that is already gone —
// killed with its group by a signal cr received — answers EPIPE, and how it
// ended is cmd.Wait's to report.
func (h *RunnerHold) Release(path string) error {
	_, _ = h.release.Write([]byte{1})
	_ = h.release.Close()
	reported, err := io.ReadAll(h.failure)
	_ = h.failure.Close()
	if err != nil {
		return fmt.Errorf("cannot read whether the test runner started: %w", err)
	}
	if len(reported) == 0 {
		return nil
	}
	errno, err := strconv.Atoi(string(reported))
	if err != nil {
		return fmt.Errorf("the held test runner reported %q", reported)
	}
	return &os.PathError{Op: "fork/exec", Path: path, Err: syscall.Errno(errno)}
}

// AwaitRunnerRelease is the first half of a held process: it waits on the
// release pipe and reports whether cr released it. On a release the pipe is
// closed and the failure pipe marked close-on-exec, so the runner the process
// becomes inherits neither.
func AwaitRunnerRelease() bool {
	release := os.NewFile(heldReleaseFD, "runner release")
	if release == nil {
		return false
	}
	var released [1]byte
	n, _ := release.Read(released[:])
	_ = release.Close()
	if n != 1 {
		return false
	}
	syscall.CloseOnExec(heldFailureFD)
	return true
}

// ReportRunnerExecFailure is the second half, reached only when exec returned:
// it sends cr the error number exec failed with.
func ReportRunnerExecFailure(err error) {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		errno = syscall.EINVAL
	}
	failure := os.NewFile(heldFailureFD, "runner failure")
	if failure == nil {
		return
	}
	_, _ = failure.WriteString(strconv.Itoa(int(errno)))
	_ = failure.Close()
}
