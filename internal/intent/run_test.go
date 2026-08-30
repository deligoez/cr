package intent

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
)

// stubTracker writes an executable script and returns the path to it, so a
// test can exercise the real invocation — the argument list, the environment,
// and the exit status — with no tracker CLI installed at all.
//
// The path is what goes into intent.cmd, and nothing here touches PATH. §3.1.1
// makes intent.cmd argv rather than a command line, so its first element is a
// program name like any other and an absolute path is a perfectly ordinary
// value for a user to configure.
func stubTracker(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tracker")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700))
	return path
}

// §3.1.2's default is the argv this package exists to run, and §3.1.1 is the
// shape it has to have. The two are asserted together because the table entry
// and the reader are one claim: a default edited into something carrying no
// placeholder would resolve perfectly well, and would fail only on the machine
// of the first person to run it.
func TestTheDefaultTrackerCommandIsTheArgvTheSpecNames(t *testing.T) {
	defaults, err := config.Resolve(config.Sources{})
	require.NoError(t, err)

	argv := defaults.Strings("intent.cmd")
	assert.Equal(t, []string{"jira", "issue", "view", "{key}", "--plain"}, argv)

	expanded, err := expand(argv, "CR-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"jira", "issue", "view", "CR-1", "--plain"}, expanded)
}

// §3.1.1 puts the placeholder in an argv element, and the element is the whole
// of the safety here. Substitution neither quotes nor splits, so every element
// arrives at the program exactly as it was written and a key nobody sanitised
// stays one argument.
//
// The key below is what makes that a claim rather than a hope. §3.2's pattern
// is overridable and --issue takes whatever the user typed, so a key carrying a
// space and a shell metacharacter is reachable, and through a shell string it
// would be a second command. Through argv it is a key that no issue matches.
func TestTheKeyIsSubstitutedIntoEveryElementThatCarriesIt(t *testing.T) {
	tracker := stubTracker(t, `for arg in "$@"; do printf '%s\n' "$arg"; done`)

	out, err := Read([]string{
		tracker, "issue", "view", Placeholder, "--jql=key = " + Placeholder, "--plain",
	}, "CR-1; rm -rf /")
	require.NoError(t, err)

	assert.Equal(t, []string{
		"issue",
		"view",
		"CR-1; rm -rf /",
		"--jql=key = CR-1; rm -rf /",
		"--plain",
	}, strings.Split(strings.TrimSuffix(out, "\n"), "\n"))
}

// §3.1.1 describes an argv array carrying a {key} placeholder, and an
// intent.cmd that is neither is refused before anything is started.
//
// The refusal has to come first, because the alternative is worse than a
// failure. An argv with no placeholder runs perfectly well and reads the same
// issue for every key it is given, so the run that follows would map claims
// from one issue onto the units of another — a coverage report built on the
// wrong specification, with nothing in it saying so.
func TestAnIntentCmdThatCannotCarryAKeyIsRefusedBeforeAnythingStarts(t *testing.T) {
	ran := filepath.Join(t.TempDir(), "ran")
	tracker := stubTracker(t, "touch "+ran)

	for name, malformed := range map[string]struct {
		argv []string
		says string
	}{
		"an empty argv":               {argv: nil, says: "empty"},
		"an argv with no placeholder": {argv: []string{tracker, "issue", "view", "CR-1"}, says: Placeholder},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := Read(malformed.argv, "CR-1")

			var refused *MalformedCommandError
			require.ErrorAs(t, err, &refused)
			assert.Empty(t, out)
			assert.Contains(t, refused.Error(), "intent.cmd")
			assert.Contains(t, refused.Error(), malformed.says)
			assert.NoFileExists(t, ran, "a misconfigured intent.cmd must cost a process, not produce one")
		})
	}
}

// §3.1.3: a non-zero exit fails, and the command's stderr reaches the user
// whole.
//
// Whole is the part worth asserting. cr knows nothing about the program it
// just started — not its flags, not its configuration, not why it declined —
// so its stderr is the entire diagnostic there is, and a summary of it would
// be cr guessing at a tool it has never heard of. Both lines survive here,
// including the one that says what to do next.
func TestAFailedTrackerCommandSurfacesItsStderr(t *testing.T) {
	tracker := stubTracker(t, "echo 'ERROR unable to authenticate: 401 Unauthorized' >&2\n"+
		"echo 'run jira init to configure a token' >&2\nexit 2")

	out, err := Read([]string{tracker, "issue", "view", Placeholder, "--plain"}, "CR-1")

	var refused *CommandError
	require.ErrorAs(t, err, &refused)
	assert.Empty(t, out)
	assert.Equal(t, "ERROR unable to authenticate: 401 Unauthorized\n"+
		"run jira init to configure a token", refused.Stderr)
	assert.Contains(t, refused.Error(), tracker+" issue view CR-1 --plain")
	assert.Contains(t, refused.Error(), refused.Stderr)

	// The exec failure stays reachable, so a caller can tell a tracker
	// command that ran and refused from one that never started at all.
	var exited *exec.ExitError
	require.ErrorAs(t, err, &exited)
	assert.Equal(t, 2, exited.ExitCode())

	silent := &CommandError{Args: []string{"jira", "issue", "view", "CR-1"}, Err: errors.New("exit status 2")}
	assert.Equal(t, "jira issue view CR-1: exit status 2", silent.Error())
}
