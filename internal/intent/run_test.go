package intent

import (
	"os"
	"path/filepath"
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
