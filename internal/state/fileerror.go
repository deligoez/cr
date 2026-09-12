package state

import "fmt"

// FileError reports a file cr had to use and could not: an input the caller
// named on the command line, a file of §2.2's tree a command requires, or a
// §2.3 write that did not land.
//
// §11.2 codes all three 3, and before this type each of them reached
// exitCodeFor as a bare fmt.Errorf and took the fallback of code 2. Measured
// 2026-09-13 against the built binary, on a pull request whose round was
// briefed: `cr record 1 missing.ndjson --repo o/r` exited 2, and so did the
// same command on a round whose state directory had no probes.ndjson. Code 2
// means the command line was wrong, so both told the user to retype an
// invocation that was right.
//
// The hint is a construction parameter and the fields are unexported, so
// §12.4's next actionable step cannot be left off: an error built any other way
// is not one of these.
type FileError struct {
	// doing is what cr was doing with the file, in the imperative the rest
	// of cr's messages use — "read", "publish", "use".
	doing string
	// path is the file.
	path string
	// hint is §12.4's next actionable step.
	hint string
	// err is what the filesystem or the decoder reported, and nil when the
	// file was read whole and cannot be used as written.
	err error
}

// FileFailure builds one.
//
// An empty hint panics rather than producing an error without one. Every call
// site passes a constant or a value derived from one, so the panic is a
// property of source that has not been run rather than of a run that failed,
// which is the cheapest moment §12.4 can be enforced at.
func FileFailure(doing, path, hint string, err error) *FileError {
	if hint == "" {
		panic("§12.4: cannot " + doing + " " + path +
			" without naming the next actionable step")
	}
	return &FileError{doing: doing, path: path, hint: hint, err: err}
}

// Error names what cr was doing, the file, and what refused. The hint is not
// repeated in it: internal/cli prints §12.4's step beside the message, and a
// step spelled twice is a step that can come to disagree with itself.
func (e *FileError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("cannot %s %s", e.doing, e.path)
	}
	return fmt.Sprintf("cannot %s %s: %v", e.doing, e.path, e.err)
}

// Hint is §12.4's next actionable step.
func (e *FileError) Hint() string { return e.hint }

// Unwrap exposes what the filesystem reported, so a caller can tell a path that
// is absent from one it may not read.
func (e *FileError) Unwrap() error { return e.err }

// UnusableHint is §12.4's next actionable step for a file of §2.2's tree cr
// found, read whole, and cannot use as written.
//
// It is separate from the hints for an absent file because the next step is: a
// file that is not there is written by a command, and a file that is there and
// undecodable is cr's own state somebody has edited.
const UnusableHint = "this is cr's own state under ~/.cr, and a hand edit is the usual cause; " +
	"repair the file or re-run the round that wrote it"
