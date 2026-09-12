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
		return fmt.Sprintf("%s: %v%s", strings.Join(e.Args, " "), e.Err, wayPast)
	}
	return fmt.Sprintf("%s: %v: %s%s", strings.Join(e.Args, " "), e.Err, e.Stderr, wayPast)
}

// wayPast names the two flags that get a run past a tracker cr cannot reach,
// and it is on the failure rather than in a document because that is where a
// reader meets the problem.
//
// It was measured rather than reasoned: a dogfood run against a scratch pull
// request had the tracker answer 404 for a key §3.2 had read off the branch
// name, and the message was the command line, the exit status, and the tool's
// own stderr — none of which says that a key cr guessed is what the tool was
// asked about. The two ways past are different fixes for different faults, so
// both are named: `--issue` corrects the key, and §3.1.4's `--intent-file`
// supplies the issue text when the tracker is unreachable whatever the key.
const wayPast = "; §3.2 resolves the key from --issue before the branch, title, and body, so " +
	"`--issue <KEY>` corrects a key cr read off the wrong one, and §3.1.4's " +
	"`--intent-file <path>` supplies the issue text without running this command at all"

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

// FileError reports an `--intent-file` that could not be read.
//
// §3.1.4 replaces the tracker command with a file, and a file cr cannot read
// leaves the run without issue text exactly as a refusing command does. §11.2
// codes both 3: it is not a validation failure, because cr never saw the
// contents to validate, and not a usage error, because the path may be
// perfectly well formed and simply absent.
type FileError struct {
	// Path is the path as `--intent-file` gave it.
	Path string
	// Err is the failure the filesystem reported.
	Err error
}

func (e *FileError) Error() string {
	return fmt.Sprintf("--intent-file %s: %v", e.Path, e.Err)
}

// Unwrap exposes the filesystem failure, so a caller can tell a path that is
// absent from one it may not read.
func (e *FileError) Unwrap() error { return e.Err }

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

// readFile reads the issue text §3.1.4 puts in a file.
//
// The bytes are returned as they were written, exactly as the command's
// standard output is. The two are interchangeable sources for one value, so
// anything done to one and not the other would make the choice of source
// visible downstream; §1.4's normalisation and §3.3's issue_hash run later,
// over whichever source produced the text.
func readFile(path string) (string, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return "", &FileError{Path: path, Err: err}
	}
	return string(text), nil
}

// Source is where one run's issue text comes from.
//
// §3.1 has two: the tracker command the user configures, and the file §3.1.4
// replaces it with. The choice between them lives here rather than at each
// call site, because §3.1.4 promises the loop works with no tracker access at
// all — and that promise is broken without ever starting a command. Resolving
// intent.cmd, validating its shape, or falling back to it when the file is
// unhelpful would each break it, and each is the sort of thing a call site
// does by reflex. One entry point taking both possibilities makes the bypass
// structural rather than a rule every future caller has to remember.
type Source struct {
	// File is the path `--intent-file` named, empty when it was not given.
	// When it is set, intent.cmd is not expanded, not validated, and not
	// started, so it need not name a program that exists.
	File string
	// Cmd is `intent.cmd` as configured, read only when File is empty.
	Cmd []string
}

// Read returns the issue text for one issue key from whichever of §3.1's two
// sources applies, the file first.
//
// The key reaches only the command. A file is the issue text already, which is
// what makes §3.1.4 a bypass rather than a cache.
func Read(source Source, key string) (string, error) {
	if source.File != "" {
		return readFile(source.File)
	}
	expanded, err := expand(source.Cmd, key)
	if err != nil {
		return "", err
	}
	return run(expanded)
}
