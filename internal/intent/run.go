// Package intent reads the issue text through the command the user configures.
//
// §3.1 makes the tracker something cr drives rather than something cr knows:
// `intent.cmd` is an argv array the user supplies, so cr carries no tracker
// authentication code of its own. Two consequences run through this file and
// are what separate it from internal/git and internal/gh, which each pin the
// program they start and the environment they hand it.
//
// The program is named at run time, because §3.1.1 makes naming it the user's
// job. It is still one program per invocation and the whole of one argv: the
// caller hands over an array, cr substitutes into its elements and starts it
// unchanged, so no argument is appended and no element is chosen.
//
// The environment is inherited whole. A tracker CLI authenticates itself, out
// of its own configuration and its own variables, and cr has never heard of
// the tool: an allowlist here would be a list of names cr cannot know, and the
// first thing it would strip is the credential the command needs to answer at
// all. §2.1.1 pays for that — the same key on two machines can read different
// issue text, and cr cannot tell — and §3.1.4's `--intent-file` is where a run
// that must be reproducible from the state directory alone goes.
package intent

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// Placeholder marks where the issue key goes.
//
// §3.1.1 puts it in an argv element and not in a shell string, and the
// difference is the whole safety of this package: substitution is per element
// and neither quotes nor splits, so a key carrying a space, a quote, or a
// shell metacharacter stays one argument and can never become a second
// command.
const Placeholder = "{key}"

// CommandError reports a tracker invocation that failed.
//
// §3.1.3 fixes the shape: a non-zero exit fails with exit code 3 and surfaces
// the command's stderr. internal/cli maps this type onto that code, and the
// stderr reaches the user whole — it is the only diagnostic there is when a
// tool cr knows nothing about refuses.
type CommandError struct {
	// Args is the expanded argv, program included, so the reported
	// command is the command that ran and can be pasted back into a
	// shell.
	Args []string
	// Stderr is what the command wrote to standard error, trimmed of
	// surrounding whitespace and otherwise untouched.
	Stderr string
	// Err is the failure os/exec reported: a non-zero exit status, or a
	// command that could not be started at all.
	Err error
}

func (e *CommandError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("%s: %v", strings.Join(e.Args, " "), e.Err)
	}
	return fmt.Sprintf("%s: %v: %s", strings.Join(e.Args, " "), e.Err, e.Stderr)
}

// Unwrap exposes the underlying exec failure, so a caller can tell a tracker
// command that ran and refused from one that never started.
func (e *CommandError) Unwrap() error { return e.Err }

// MalformedCommandError reports an `intent.cmd` that §3.1.1 cannot accept.
//
// It is a configuration failure rather than a command failure — nothing was
// started — but §11.2 gives the two the same code, so a caller need not tell
// them apart to exit correctly.
type MalformedCommandError struct {
	// Args is `intent.cmd` as configured, before any substitution.
	Args []string
	// Reason says what is wrong with it and what to do about it.
	Reason string
}

func (e *MalformedCommandError) Error() string {
	return fmt.Sprintf("intent.cmd %v: %s", e.Args, e.Reason)
}

// expand substitutes key into every element of argv that carries the
// placeholder, and refuses an argv §3.1.1 does not describe.
//
// The refusal happens before anything is started, so a misconfigured
// `intent.cmd` costs a process rather than producing one that read the wrong
// issue.
func expand(argv []string, key string) ([]string, error) {
	if len(argv) == 0 {
		return nil, &MalformedCommandError{
			Args:   slices.Clone(argv),
			Reason: "empty; §3.1.1 requires an argv array whose first element names the tracker command, or pass --intent-file to bypass it",
		}
	}

	expanded := make([]string, 0, len(argv))
	placed := false
	for _, arg := range argv {
		if strings.Contains(arg, Placeholder) {
			placed = true
		}
		expanded = append(expanded, strings.ReplaceAll(arg, Placeholder, key))
	}
	if !placed {
		return nil, &MalformedCommandError{
			Args:   slices.Clone(argv),
			Reason: "no " + Placeholder + " placeholder; §3.1.1 requires one, and without it every issue key would read the same issue",
		}
	}
	return expanded, nil
}

// run starts one tracker invocation and returns its standard output.
//
// argv is expanded and non-empty, expand being the only way to obtain one.
// Stdin is left nil, which os/exec connects to the null device, so a command
// that would prompt reads EOF and fails rather than waiting on a terminal
// nobody is watching.
func run(argv []string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = os.Environ()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", &CommandError{
			Args:   slices.Clone(argv),
			Stderr: strings.TrimSpace(stderr.String()),
			Err:    err,
		}
	}
	return stdout.String(), nil
}

// Read runs the configured tracker command for one issue key and returns the
// issue text it printed.
func Read(argv []string, key string) (string, error) {
	expanded, err := expand(argv, key)
	if err != nil {
		return "", err
	}
	return run(expanded)
}
