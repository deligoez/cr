package intent

import (
	"os"
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
