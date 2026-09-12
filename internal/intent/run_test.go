package intent

import (
	"errors"
	"io/fs"
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

	out, err := Read(Source{Cmd: []string{
		tracker, "issue", "view", Placeholder, "--jql=key = " + Placeholder, "--plain",
	}}, "CR-1; rm -rf /")
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
			out, err := Read(Source{Cmd: malformed.argv}, "CR-1")

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

	out, err := Read(Source{Cmd: []string{tracker, "issue", "view", Placeholder, "--plain"}}, "CR-1")

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
	assert.Equal(t, "jira issue view CR-1: exit status 2"+wayPast, silent.Error())
}

// A tracker that refused names the two flags that get a run past it, whether or
// not it said anything itself.
//
// §12.4 has every error name the next actionable step, and this one could not:
// the tool's stderr is about the tool, and the fault is as often cr's key as
// the tracker's state. A dogfood run met exactly that — a 404 for a key read
// off the branch name — and neither flag appeared anywhere in the failure.
func TestAFailedTrackerCommandNamesTheFlagsThatGetPastIt(t *testing.T) {
	for name, refused := range map[string]*CommandError{
		"a tracker that explained itself": {
			Args: []string{"jira", "issue", "view", "CR-1"},
			Err:  errors.New("exit status 2"), Stderr: "404 not found",
		},
		"a tracker that said nothing": {
			Args: []string{"jira", "issue", "view", "CR-1"}, Err: errors.New("exit status 2"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, refused.Error(), "--issue <KEY>",
				"§3.2: the key cr resolved is as likely the fault as the tracker")
			assert.Contains(t, refused.Error(), "--intent-file <path>",
				"§3.1.4: the issue text without the command at all")
		})
	}
}

// The tracker command is handed the ambient environment whole, which is the
// one place this runner deliberately departs from internal/git and
// internal/gh.
//
// §3.1 says cr carries no tracker authentication code, and the only way that
// can be true is if the command authenticates itself. A tracker CLI does that
// out of its own configuration and its own variables — the very names an
// allowlist strips — and cr cannot enumerate them, because it has never heard
// of the tool. git and gh are cr's own tools and cr knows exactly which of
// their variables redirect a read; here there is nothing to know.
//
// §2.1.1 is what that costs. The same key on two machines can read different
// issue text, and cr cannot tell, so a run is reproducible given the same
// environment rather than given the state directory alone. §3.1.4's
// --intent-file is where a run that needs the stronger guarantee goes.
func TestTheTrackerCommandKeepsTheEnvironmentItAuthenticatesWith(t *testing.T) {
	seen := filepath.Join(t.TempDir(), "environment")
	tracker := stubTracker(t, "env > "+seen)
	t.Setenv("JIRA_API_TOKEN", "the-users-own-token")
	t.Setenv("JIRA_AUTH_TYPE", "bearer")

	_, err := Read(Source{Cmd: []string{tracker, "issue", "view", Placeholder, "--plain"}}, "CR-1")
	require.NoError(t, err)

	environment, err := os.ReadFile(seen)
	require.NoError(t, err)
	assert.Contains(t, string(environment), "JIRA_API_TOKEN=the-users-own-token")
	assert.Contains(t, string(environment), "JIRA_AUTH_TYPE=bearer")
}

// issueFile writes issue text where --intent-file can point at it, and
// returns the path.
func issueFile(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	return path
}

// §3.1.4: --intent-file bypasses the command and reads the issue text from a
// file.
//
// Bypass is the word under test, and it is not the same claim as the returned
// text being the file's. A run that started the tracker and then preferred the
// file would satisfy every assertion about what came back and none of what
// §3.1.4 is for: the command would still have authenticated, still have
// reached the network, and still have failed on a machine with no tracker
// access. So the stub records having run at all, and the assertion is that the
// record is not there.
func TestAnIntentFileBypassesTheTrackerCommandEntirely(t *testing.T) {
	ran := filepath.Join(t.TempDir(), "ran")
	tracker := stubTracker(t, "touch "+ran+"\necho 'text the tracker would have printed'")
	issue := "CR-1 Bypass the tracker with an intent file\n\n" +
		"Acceptance: the loop works with no tracker access at all.\n"

	text, err := Read(Source{
		File: issueFile(t, issue),
		Cmd:  []string{tracker, "issue", "view", Placeholder, "--plain"},
	}, "CR-1")

	require.NoError(t, err)
	assert.Equal(t, issue, text)
	assert.NoFileExists(t, ran,
		"§3.1.4 bypasses the command; it does not run it and discard the output")
}

// §3.1.4's promise is that the loop works with no tracker access **at all**,
// and "at all" reaches past the command not being started: nothing about
// intent.cmd may have to hold for the run to succeed.
//
// Each row is an intent.cmd that fails a different way, and every one of them
// is reachable. §3.1.2's default is what every user has until they configure
// otherwise, so a bypass that resolved the command first would need jira
// installed to read an issue from a file. The two argv shapes below it are the
// ones expand refuses, so a bypass that validated before choosing would refuse
// them too — and refuse a run that was never going to use them.
func TestAnIntentFileNeedsNoTrackerCommandThatCouldEverRun(t *testing.T) {
	defaults, err := config.Resolve(config.Sources{})
	require.NoError(t, err)

	issue := "CR-2 The tracker is unreachable from here\n"
	path := issueFile(t, issue)
	absent := filepath.Join(t.TempDir(), "jira")
	require.NoFileExists(t, absent)

	for name, argv := range map[string][]string{
		"§3.1.2's default, naming a program cr does not ship": defaults.Strings("intent.cmd"),
		"a program that does not exist":                       {absent, "issue", "view", Placeholder},
		"an argv expand refuses for carrying no placeholder":  {absent, "issue", "view", "CR-2"},
		"no intent.cmd configured at all":                     nil,
	} {
		t.Run(name, func(t *testing.T) {
			text, err := Read(Source{File: path, Cmd: argv}, "CR-2")

			require.NoError(t, err)
			assert.Equal(t, issue, text)
		})
	}
}

// A file cr cannot read fails the run, and fails it as a file.
//
// Failing is the decision worth pinning. §3.2 already has a path where no
// issue key is found and cr continues with an empty intent, and a mistyped
// --intent-file quietly taking that path would produce a coverage report
// claiming the issue asked for nothing — every unit unmapped, every question
// raised against a specification that was on disk the whole time. §3.1.4 makes
// the file the tracker command's replacement, so it fails the way the command
// it replaced would have: §11.2 codes the file and the external command in one
// row, and internal/cli maps this type onto it.
func TestAnUnreadableIntentFileFailsAsAFileFailure(t *testing.T) {
	directory := t.TempDir()
	missing := filepath.Join(directory, "issue.txt")

	text, err := Read(Source{
		File: missing,
		Cmd:  []string{"jira", "issue", "view", Placeholder, "--plain"},
	}, "CR-3")

	var unreadable *FileError
	require.ErrorAs(t, err, &unreadable)
	assert.Empty(t, text)
	assert.Equal(t, missing, unreadable.Path)
	assert.Contains(t, unreadable.Error(), "--intent-file "+missing)
	assert.ErrorIs(t, err, fs.ErrNotExist,
		"the filesystem's own failure stays reachable, so a caller can tell absent from unreadable")

	// A path that exists and still yields no issue text fails the same way,
	// so the check is the read failing rather than a special case for a
	// file that is not there.
	_, err = Read(Source{File: directory}, "CR-3")
	require.ErrorAs(t, err, &unreadable)
	assert.Equal(t, directory, unreadable.Path)
	assert.NotErrorIs(t, err, fs.ErrNotExist)
}
