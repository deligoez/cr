package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

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
