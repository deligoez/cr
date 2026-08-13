// Package git reads the repository under review through the git command line.
//
// Every invocation in this package is a read. §2.2 permits cr exactly one
// write inside the repository under review — the worktree registration of
// §5.1 — and this is not where it happens: nothing here touches the index,
// HEAD, a branch, the stash, or a tracked file.
//
// A run is pinned rather than inherited. §2.1.1 requires the same state, the
// same head, and the same inputs to produce the same result, and git reads a
// great deal of ambient configuration and environment that would otherwise
// decide what a diff looks like — up to and including replacing the diff
// engine outright with `diff.external`.
package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// CommandError reports a git invocation that failed.
//
// §3.1.3 fixes the shape for an external command: a non-zero exit fails with
// exit code 3 and surfaces the command's stderr. git is one of the external
// commands cr drives, so it fails in that shape too rather than in one of its
// own, and internal/cli maps this type onto that code.
type CommandError struct {
	// Args are the arguments git was given, the binary name aside. They
	// include the pinning of run, so the reported command is the command
	// that ran and can be pasted back into a shell.
	Args []string
	// Stderr is what git wrote to standard error, trimmed of surrounding
	// whitespace. §3.1.3 requires it to reach the user.
	Stderr string
	// Err is the failure os/exec reported: a non-zero exit status, or a
	// git that could not be started at all.
	Err error
}

func (e *CommandError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("git %s: %v", strings.Join(e.Args, " "), e.Err)
	}
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.Args, " "), e.Err, e.Stderr)
}

// Unwrap exposes the underlying exec failure, so a caller can tell a git that
// ran and refused from a git that never started.
func (e *CommandError) Unwrap() error { return e.Err }

// pinnedConfig are the `-c` overrides every run carries.
//
// A `-c` option outranks the system, the global, and the repository
// configuration files alike, which is why cr pins the settings that decide the
// output instead of discarding the configuration wholesale: dropping it would
// take `safe.directory` and everything else a working checkout depends on with
// it. Every knob that has a command-line flag is pinned by that flag in
// diffArgs; these are the ones that have none.
var pinnedConfig = []string{
	// Paths as raw UTF-8 rather than octal escapes, so a non-ASCII path
	// reaches the caller spelled the way the repository spells it.
	"core.quotepath=false",
	// The indent heuristic shifts hunk boundaries, so it decides which
	// lines a §3.4 unit ends up containing.
	"diff.indentHeuristic=true",
	// A blank context line is written as one empty line either way.
	"diff.suppressBlankEmpty=false",
	// Rename detection gives up above this many candidates, so the limit
	// decides whether a rename is reported as a rename.
	"diff.renameLimit=1000",
}

// pinnedEnv is the environment every run carries, ahead of the inherited
// names, so it wins on the duplicate key.
var pinnedEnv = []string{
	// The C locale, so git's own messages and anything it formats or
	// sorts by locale read the same on every machine.
	"LC_ALL=C",
	"LANG=C",
	// No pager: on a captured read it would either swallow the output or
	// block on a terminal that is not attached.
	"GIT_PAGER=cat",
	// No prompt, ever. A read that cannot proceed must fail and say so
	// rather than wait on a human who may not be watching.
	"GIT_TERMINAL_PROMPT=0",
	// No optional locks. git refreshes the index opportunistically on
	// commands that do not need it, and invariant 2 forbids writing
	// inside the repository under review at all.
	"GIT_OPTIONAL_LOCKS=0",
}

// inherited is the whole of the ambient environment a run is given.
//
// It is an allowlist and not a filter: GIT_DIR, GIT_WORK_TREE,
// GIT_INDEX_FILE, GIT_EXTERNAL_DIFF, GIT_CONFIG and their neighbours would
// each point a run at something other than the repository cr was asked about,
// and a list of the names to drop is a list that goes stale as git grows.
var inherited = []string{
	// git resolves its own subcommands and helpers through PATH.
	"PATH",
	// The global configuration lives under HOME, and `safe.directory`
	// with it: a checkout that CI marked safe must stay safe.
	"HOME",
	// Where git places its temporary files.
	"TMPDIR",
	// Windows: git cannot start without it.
	"SystemRoot",
}

// environ builds the environment for one git read.
func environ() []string {
	env := slices.Clone(pinnedEnv)
	for _, name := range inherited {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

// run executes one git read inside dir and returns its standard output.
func run(dir string, args ...string) (string, error) {
	full := []string{"-C", dir}
	for _, setting := range pinnedConfig {
		full = append(full, "-c", setting)
	}
	full = append(full, args...)

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", full...)
	cmd.Env = environ()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", &CommandError{
			Args:   full,
			Stderr: strings.TrimSpace(stderr.String()),
			Err:    err,
		}
	}
	return stdout.String(), nil
}
