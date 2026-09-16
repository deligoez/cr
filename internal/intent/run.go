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

	"github.com/deligoez/cr/internal/note"
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
	// Resolved is whether the key the command was asked about is the one
	// §3.2 resolved in this run, which Resolve sets. It decides which flags
	// the message may name: `--issue` corrects a key the running command
	// resolved, and a command reading the key a round recorded — `cr claims
	// record` — has no such flag, so naming it there sends the reader to a
	// flag the command refuses.
	Resolved bool
}

func (e *CommandError) Error() string {
	past := wayPastRecorded
	if e.Resolved {
		past = wayPast
	}
	if e.Stderr == "" {
		return fmt.Sprintf("%s: %v%s", strings.Join(e.Args, " "), e.Err, past)
	}
	return fmt.Sprintf("%s: %v: %s%s", strings.Join(e.Args, " "), e.Err, e.Stderr, past)
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

// wayPastRecorded is wayPast for a command that reads the key its round
// recorded rather than resolving one, and so names only the flag such a
// command accepts. Release QA met the difference: `cr claims record` failed on
// a 404 and the message named `--issue <KEY>`, which that command does not have.
const wayPastRecorded = "; §3.1.4's `--intent-file <path>` supplies the issue text " +
	"without running this command at all"

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

// FileError reports an intent file that could not be read.
//
// §3.1.4 replaces the tracker command with a file, and a file cr cannot read
// leaves the run without issue text exactly as a refusing command does. §11.2
// codes both 3: it is not a validation failure, because cr never saw the
// contents to validate, and not a usage error, because the path may be
// perfectly well formed and simply absent.
//
// §3.1.5's extra intent files are read through the same type rather than
// through one of their own. The fault is identical — a path the user named and
// cr could not read — and so is the code; what differs is the flag to correct,
// which Flag carries so the message names the one the user typed.
type FileError struct {
	// Path is the path as the flag or the configuration gave it.
	Path string
	// Flag is the flag the path came from, `--intent-file` when it is
	// empty. `intent.extra_files` supplies paths too, and a path from it
	// is reported against `--intent-extra`: the two are one list per
	// §3.1.5, and the flag is what the reader can act on immediately.
	Flag string
	// Err is the failure the filesystem reported.
	Err error
}

func (e *FileError) Error() string {
	flag := e.Flag
	if flag == "" {
		flag = "--intent-file"
	}
	return fmt.Sprintf("%s %s: %v", flag, e.Path, e.Err)
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
func readFile(path, flag string) (string, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return "", &FileError{Path: path, Flag: flag, Err: err}
	}
	return string(text), nil
}

// ExtraFlag is the flag §3.1.5 names an extra intent file with, carried on the
// failure to read one so the message names what the user can correct.
const ExtraFlag = "--intent-extra"

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
	// Extra is §3.1.5's extra intent files, in the order their texts are
	// appended: the `--intent-extra` paths, then the `intent.extra_files`
	// paths, each path once, as intent.ExtraFiles orders them.
	//
	// It is read whichever of the two sources above produced the first
	// part. §3.1.4 bypasses the tracker command and nothing else, so a
	// run reading its issue text from a file reads the same extra files a
	// run reading it from the tracker does, and a claim drawn from one is
	// checked against the same text either way.
	Extra []string
}

// Reading is one read of the issue text.
type Reading struct {
	// Text is the issue text cr stores, prints, and checks a span against:
	// what the source produced, with Clean applied, and §3.1.5's extra
	// intent files appended to it under their separator lines.
	Text string
	// asRead is what the sources produced before Clean, joined the same
	// way, and empty when Clean changed nothing. It is kept for one
	// purpose, DetectDrift's: a claim recorded before Clean existed
	// carries an `issue_hash` taken over these bytes, and an unchanged
	// issue must not read as drifted because cr began cleaning it.
	asRead string
}

// Spans pairs this reading with the context store, as §3.3 checks claims
// against the two.
func (r Reading) Spans(notes []note.Note) SpanTexts {
	return SpanTexts{Issue: r.Text, Notes: notes, asRead: r.asRead}
}

// Links is Links over this reading: every URL Text carries, and the target of
// every terminal hyperlink Clean removed on the way to Text, in the order each
// first appears in what the sources produced, each once.
//
// The extra intent files of §3.1.5 are scanned with the tracker's text,
// because §3.1.7's reason for the report does not distinguish them: cr read
// the text it was given and not what any of it links to, whichever part the
// link sits in.
func (r Reading) Links() []string {
	raw := r.asRead
	if raw == "" {
		raw = r.Text
	}
	return Links(clean(raw, true))
}

// read wraps the bytes the sources produced as a Reading: the base text, then
// §3.1.5's extra intent files in the order they were read, each carrying the
// path as it was given.
//
// Each part is cleaned on its own and the separator lines are written from the
// paths as given, so §3.1.6's cleanup reaches every source of §3.1.1 through
// §3.1.5 and reaches nothing cr itself wrote.
func read(raw string, extras ...Part) Reading {
	cleanedExtras := make([]Part, 0, len(extras))
	for _, extra := range extras {
		cleanedExtras = append(cleanedExtras, Part{File: extra.File, Text: Clean(extra.Text)})
	}
	cleaned, joined := join(Clean(raw), cleanedExtras), join(raw, extras)
	if cleaned == joined {
		return Reading{Text: cleaned}
	}
	return Reading{Text: cleaned, asRead: joined}
}

// readExtras reads §3.1.5's extra intent files, in the order they were given.
//
// A file that cannot be read fails the run rather than being skipped: a part
// silently missing from the issue text is a part no claim can be drawn from
// and one every recorded claim of it reads as drifted, and neither says that a
// path was wrong.
func readExtras(paths []string) ([]Part, error) {
	extras := make([]Part, 0, len(paths))
	for _, path := range paths {
		body, err := readFile(path, ExtraFlag)
		if err != nil {
			return nil, err
		}
		extras = append(extras, Part{File: path, Text: body})
	}
	return extras, nil
}

// Read returns the issue text for one issue key: whichever of §3.1's two
// sources applies for the first part, the file first, followed by §3.1.5's
// extra intent files.
//
// The key reaches only the command. A file is the issue text already, which is
// what makes §3.1.4 a bypass rather than a cache.
//
// Both sources are cleaned alike. They are interchangeable sources for one
// value, so a file holding what the tracker command printed must yield the
// text the command would have, or the choice of source would be visible in
// every span and every hash downstream.
//
// The first part is read before any extra file, and every extra file before
// anything is joined, so a run that fails reads no issue text at all rather
// than a truncated one.
func Read(source Source, key string) (Reading, error) {
	raw, err := firstPart(source, key)
	if err != nil {
		return Reading{}, err
	}
	extras, err := readExtras(source.Extra)
	if err != nil {
		return Reading{}, err
	}
	return read(raw, extras...), nil
}

// firstPart is the text of §3.1.1 through §3.1.4: the file `--intent-file`
// named, or what the tracker command printed.
func firstPart(source Source, key string) (string, error) {
	if source.File != "" {
		return readFile(source.File, "")
	}
	expanded, err := expand(source.Cmd, key)
	if err != nil {
		return "", err
	}
	return run(expanded)
}
