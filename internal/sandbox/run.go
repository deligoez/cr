package sandbox

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// SetupError reports a `sandbox.setup` command that failed.
//
// §3.1.3 fixes the shape for an external command cr drives: a non-zero exit
// fails with exit code 3 and the command's stderr reaches the user. A setup
// command is such a command — the profile names it and cr has never heard of
// it — so it fails in that shape rather than in one of its own, and
// internal/cli maps this type onto that code.
type SetupError struct {
	// Args is the command as it ran, program included, so what is
	// reported can be pasted back into a shell.
	Args []string
	// Stderr is what the command wrote to standard error, trimmed of
	// surrounding whitespace. It is the only diagnostic there is when a
	// tool cr knows nothing about refuses.
	Stderr string
	// Err is the failure os/exec reported: a non-zero exit status, or a
	// command that could not be started at all.
	Err error
}

func (e *SetupError) Error() string {
	ran := "sandbox.setup " + strings.Join(e.Args, " ")
	if e.Stderr == "" {
		return fmt.Sprintf("%s: %v", ran, e.Err)
	}
	return fmt.Sprintf("%s: %v: %s", ran, e.Err, e.Stderr)
}

// Unwrap exposes the underlying exec failure, so a caller can tell a command
// that ran and refused from one that never started.
func (e *SetupError) Unwrap() error { return e.Err }

// RunError reports a test runner that could not be started at all.
//
// It is the failure that is not a test result. A runner that ran and exited
// non-zero has said something about the code — §5.2.4 records that exit code and
// §5.3.4's ladder reads it — while a runner that never started has said nothing,
// and §3.1.3 gives that the shape every external command cr drives fails in:
// exit code 3, with what the attempt reported.
type RunError struct {
	// Args is the command as it was to have run, program included.
	Args []string
	// Err is what os/exec reported.
	Err error
}

func (e *RunError) Error() string {
	return fmt.Sprintf("cannot run %s: %v", strings.Join(e.Args, " "), e.Err)
}

// Unwrap exposes the underlying failure.
func (e *RunError) Unwrap() error { return e.Err }

// Run runs the profile's test command inside the sandbox and returns the
// runner's own exit status (§5.2.1).
//
// dir is the sandbox, and it is the whole point of the function. A suite run in
// the main checkout would be measuring the user's working tree — mid-edit,
// possibly on another branch — and reporting the answer as if it came from the
// pull request head, while §2.2 forbids cr to have put the probe there in the
// first place. So the directory is pinned here and comes from state.Layout by
// way of §5.1.6's check, never from a caller's own join.
//
// The output goes to log as it is produced rather than being returned.
// §5.2.1 also asks for the exit code, the duration and a bounded tail of the
// output to be *recorded*, and that record is §5.2.4's; what a person watching
// the command wants meanwhile is to see the suite run.
//
// A non-zero exit is returned as a value, not as an error. A failing suite is an
// ordinary and often intended outcome — §5.3.4's whole ladder is built on
// reading one — so the number is reported and its meaning is left to the section
// that owns it.
//
// The environment is inherited whole, for the reason runSetup inherits it: the
// runner is a tool the user names and cr has never heard of, and an allowlist
// here would be a list of names cr cannot know.
func Run(argv []string, dir string, log io.Writer) (int, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.Stdout = log
	cmd.Stderr = log
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exit):
		return exit.ExitCode(), nil
	}
	return 0, &RunError{Args: slices.Clone(argv), Err: err}
}

// setupArgv splits one `sandbox.setup` entry into the argv it runs as.
//
// There is no shell. §3.1.1 already settled the same question for the tracker
// command — an argv array, substituted per element, so nothing quotes and
// nothing splits — and a shell here would buy a run whose result depends on
// which shell is installed and on what the ambient environment expands to,
// which §2.1.1 asks cr not to have. A profile that needs a shell writes one:
// `sh /path/to/setup.sh` is a command like any other.
//
// An entry that splits to nothing is not a command, and is reported by the
// caller rather than skipped.
func setupArgv(command string) []string {
	return strings.Fields(command)
}

// runSetup runs one setup command in dir, which is §5.1.3's sandbox root.
//
// The environment is inherited whole, for the reason internal/intent inherits
// it: the command is a tool the user names and cr has never heard of —
// `composer`, `npm`, `make` — and an allowlist here would be a list of names cr
// cannot know, the first casualty being whatever credential or cache path the
// tool needs to work at all. What cr does pin is the directory, because §5.1.3
// says where the command runs.
//
// Standard output is discarded and standard error kept. §5.1.3 asks for the
// commands to run, not for their output to be recorded — that is §5.2.4's
// business, for runs that evidence a finding — but a command that failed has
// to be able to say why.
func runSetup(argv []string, dir string) error {
	var stderr bytes.Buffer
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return &SetupError{
			Args:   slices.Clone(argv),
			Stderr: strings.TrimSpace(stderr.String()),
			Err:    err,
		}
	}
	return nil
}
