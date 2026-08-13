// Package gh reads the pull request under review through the gh command line.
//
// Every invocation in this package is a read. §8.5 requires --confirm for
// every network write, and a read that happens during orientation is nowhere
// near that gate: §3.7 has cr brief perform no network write at all. Nothing
// here posts a comment, resolves a thread, or edits anything on GitHub.
//
// A run is pinned rather than inherited, for the same reason internal/git
// pins its own. §2.1.1 requires the same state, the same head, and the same
// inputs to produce the same result, and gh reads a great deal of ambient
// environment that would otherwise decide which server answers.
package gh

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// CommandError reports a gh invocation that failed.
//
// §3.1.3 fixes the shape for an external command: a non-zero exit fails with
// exit code 3 and surfaces the command's stderr. gh is one of the external
// commands cr drives, so it fails in that shape too rather than in one of its
// own, and internal/cli maps this type onto that code. A GraphQL error comes
// back the same way: the API answers 200 with an `errors` array, gh reports
// the message on stderr and exits non-zero, so a query that named a pull
// request nobody can see is a CommandError like any other refusal.
type CommandError struct {
	// Args are the arguments gh was given, the binary name aside, so the
	// reported command is the command that ran and can be pasted back
	// into a shell.
	Args []string
	// Stderr is what gh wrote to standard error, trimmed of surrounding
	// whitespace. §3.1.3 requires it to reach the user.
	Stderr string
	// Err is the failure os/exec reported: a non-zero exit status, or a
	// gh that could not be started at all.
	Err error
}

func (e *CommandError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("gh %s: %v", strings.Join(e.Args, " "), e.Err)
	}
	return fmt.Sprintf("gh %s: %v: %s", strings.Join(e.Args, " "), e.Err, e.Stderr)
}

// Unwrap exposes the underlying exec failure, so a caller can tell a gh that
// ran and refused from a gh that never started.
func (e *CommandError) Unwrap() error { return e.Err }

// pinnedEnv is the environment every run carries.
var pinnedEnv = []string{
	// The C locale, so gh's own messages read the same on every machine.
	"LC_ALL=C",
	"LANG=C",
	// No pager: on a captured read it would either swallow the output or
	// block on a terminal that is not attached.
	"GH_PAGER=cat",
	"PAGER=cat",
	// No colour. The output being parsed is JSON, and escape codes in the
	// middle of it are not.
	"NO_COLOR=1",
	// No prompt, ever. A read that cannot proceed must fail and say so
	// rather than wait on a human who may not be watching.
	"GH_PROMPT_DISABLED=1",
	// No release check. It is a second network call on an unrelated host
	// whose notice lands on stderr, where a failure's message belongs.
	"GH_NO_UPDATE_NOTIFIER=1",
}

// inherited is the whole of the ambient environment a run is given.
//
// It is an allowlist and not a filter, and the names it leaves out are the
// point. GH_HOST, GH_ENTERPRISE_TOKEN, GH_TOKEN, GH_REPO, GH_CONFIG_DIR and
// XDG_CONFIG_HOME each decide which server answers, or under whose
// credentials: a read redirected at another host resolves the same owner and
// name to a different repository altogether, and cr would then ingest a
// stranger's threads as the ones on the pull request under review.
// Suppressing a finding against a thread nobody on this pull request ever
// wrote is the expensive kind of wrong. The host is therefore never ambient:
// it stays gh's documented default until an invocation names another one.
//
// The cost is deliberate: a machine that authenticates only through GH_TOKEN
// in the environment has no credentials here and gh says so, which is a
// failure that names itself rather than a read that quietly went elsewhere.
// Authentication comes from gh's own configuration under HOME.
var inherited = []string{
	// gh resolves git and its own helpers through PATH.
	"PATH",
	// gh's configuration and credentials live under HOME.
	"HOME",
	// Where gh places its temporary files.
	"TMPDIR",
	// Windows: gh cannot start without it.
	"SystemRoot",
}

// environ builds the environment for one gh read.
func environ() []string {
	env := slices.Clone(pinnedEnv)
	for _, name := range inherited {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

// Runner executes one gh invocation and returns its standard output.
//
// It is the seam the transport sits behind. Ingestion is then testable
// against recorded payloads with no network call, no token, and no rate
// limit, which is the only way a test of it can be honest about what it
// proves.
type Runner func(args ...string) (string, error)

// Run executes one gh read and returns its standard output.
func Run(args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("gh", args...)
	cmd.Env = environ()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", &CommandError{
			Args:   slices.Clone(args),
			Stderr: strings.TrimSpace(stderr.String()),
			Err:    err,
		}
	}
	return stdout.String(), nil
}
