package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creack/pty"

	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// crHome points the state tree of spec/0.1.0.md §2.2 at a temporary directory,
// so `cr config` resolves the built-in defaults and reads nothing of the
// author's own.
func crHome(t *testing.T) {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".cr")
	require.NoError(t, os.MkdirAll(root, 0o700))
	t.Setenv(state.HomeEnv, root)
}

// execute runs one command with its output on file.
//
// The file is the whole point. §12.1 keys the output shape on what stdout is,
// so a test that hands the command a buffer and a flag saying what to pretend
// about it proves that the branch runs and nothing about the detection. Every
// caller here passes a real open file, and the answer comes from the kernel.
func execute(t *testing.T, file *os.File, args ...string) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetOut(file)
	cmd.SetErr(file)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
}

// throughAPipe runs a command with stdout on a real pipe and returns what came
// out of the other end. This is how an agent runs cr.
func throughAPipe(t *testing.T, args ...string) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { reader.Close() })

	// Drained concurrently, so a command whose output outgrows the pipe
	// buffer cannot deadlock the test it is being read by.
	drained := make(chan string, 1)
	go func() {
		read, _ := io.ReadAll(reader)
		drained <- string(read)
	}()

	execute(t, writer, args...)
	require.NoError(t, writer.Close())
	return <-drained
}

// throughATerminal runs a command with stdout on the slave side of a real
// pseudo-terminal, which is the one way to prove the detection rather than the
// branch behind it. A boolean the test sets itself would exercise the same
// line of code and establish nothing about what happens in front of a person.
func throughATerminal(t *testing.T, args ...string) string {
	t.Helper()
	master, slave, err := pty.Open()
	require.NoError(t, err)
	t.Cleanup(func() { master.Close() })

	drained := make(chan string, 1)
	go func() {
		// The master reports EIO rather than EOF when the last slave
		// closes, after handing over everything already written.
		read, _ := io.ReadAll(master)
		drained <- string(read)
	}()

	execute(t, slave, args...)
	require.NoError(t, slave.Close())
	// A terminal's line discipline turns \n into \r\n on the way out.
	return strings.ReplaceAll(<-drained, "\r\n", "\n")
}

// §12.1: output is JSON when stdout is not a terminal. No flag is needed to
// reach it, which is the half of the rule that matters most — cr's caller is an
// agent reading a pipe, and the shape it parses is the one it gets by default.
func TestAPipedCommandEmitsJSON(t *testing.T) {
	crHome(t)

	out := throughAPipe(t, "config")

	var printed map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &printed), "a pipe was given %q", out)
	assert.Equal(t, float64(20), printed["post.max_comments"])
	assert.NotContains(t, out, "\x1b[", "an escape sequence in a JSON document is a defect in the document")
}

// §12.1's other half: a terminal is given human-readable coloured text, and it
// is the only thing that is. The colour is asserted rather than assumed,
// because a rendering that reads the flag and then never uses the answer is
// indistinguishable from one that got the flag wrong.
func TestATerminalCommandEmitsColouredText(t *testing.T) {
	crHome(t)

	out := throughATerminal(t, "config")

	assert.False(t, json.Valid([]byte(out)), "a terminal was given JSON: %q", out)
	assert.Contains(t, out, "\x1b[36mpost.max_comments\x1b[0m = 20")
}

// §12.1 gives `--json` one job, and it is the only override there is: a
// terminal that would have been given text is given JSON instead. Nothing
// forces the other direction, so this is the whole of the flag.
func TestTheJSONFlagForcesJSONOnATerminal(t *testing.T) {
	crHome(t)

	out := throughATerminal(t, "config", "--json")

	var printed map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &printed), "a terminal under --json was given %q", out)
	assert.Equal(t, float64(20), printed["post.max_comments"])
	assert.NotContains(t, out, "\x1b[", "colour belongs to the text rendering and to nothing else")
}
