package cli

import (
	"bytes"
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
// so a command resolves the built-in defaults and reads nothing of the author's
// own. It returns the root, which is what `cr init` reports.
func crHome(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".cr")
	require.NoError(t, os.MkdirAll(root, 0o700))
	t.Setenv(state.HomeEnv, root)
	return root
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

// terminalSentinel is written to the pseudo-terminal after the command has
// finished, so the reader knows it has everything without waiting for the file
// to close. It is not a value any command produces.
const terminalSentinel = "cr-output-drained"

// throughATerminal runs a command with stdout on the slave side of a real
// pseudo-terminal, which is the one way to prove the detection rather than the
// branch behind it. A boolean the test sets itself would exercise the same
// line of code and establish nothing about what happens in front of a person.
//
// The sentinel is what makes the read deterministic. Closing the slave and
// reading to the end is the obvious shape and it is a race: the master reports
// EIO the moment the last slave goes, and output still sitting in the terminal
// buffer is lost with it, so the test passes or reports nothing at all
// depending on which side got there first. Reading concurrently keeps a command
// larger than the buffer from deadlocking, and stopping at the sentinel keeps
// it from depending on a close at all.
func throughATerminal(t *testing.T, args ...string) string {
	t.Helper()
	master, slave, err := pty.Open()
	require.NoError(t, err)
	t.Cleanup(func() { master.Close() })
	t.Cleanup(func() { slave.Close() })

	drained := make(chan string, 1)
	go func() {
		var read []byte
		chunk := make([]byte, 4096)
		for {
			n, err := master.Read(chunk)
			read = append(read, chunk[:n]...)
			if err != nil || bytes.Contains(read, []byte(terminalSentinel)) {
				break
			}
		}
		drained <- string(read)
	}()

	execute(t, slave, args...)
	_, err = slave.WriteString(terminalSentinel + "\n")
	require.NoError(t, err)

	// A terminal's line discipline turns \n into \r\n on the way out.
	out := strings.ReplaceAll(<-drained, "\r\n", "\n")
	out, _, ok := strings.Cut(out, terminalSentinel)
	require.True(t, ok, "the terminal never handed back the sentinel; it gave %q", out)
	return out
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

// §11.1 gives `--no-color` the colour and nothing beyond it. There is no flag
// that forces text, so the shape is unchanged in both directions: a pipe under
// `--no-color` is still JSON, and a terminal under it is still text.
//
// The two halves are one test because the claim is about the boundary between
// them. A run that answered either half alone would be consistent with
// `--no-color` having quietly become a second way to ask for text.
func TestNoColorStripsColourAndNothingElse(t *testing.T) {
	crHome(t)

	piped := throughAPipe(t, "config", "--no-color")
	assert.True(t, json.Valid([]byte(piped)), "a pipe under --no-color was given %q", piped)

	terminal := throughATerminal(t, "config", "--no-color")
	assert.False(t, json.Valid([]byte(terminal)), "a terminal under --no-color was given JSON: %q", terminal)
	assert.Contains(t, terminal, "post.max_comments = 20")
	assert.NotContains(t, terminal, "\x1b[", "--no-color left an escape sequence behind")
}

// §12.2: the JSON is pretty-printed with two-space indentation.
//
// Re-indenting what cr printed has to be a no-op, which is a stronger claim
// than reading one line off the top: it holds at every depth the document has,
// and it fails for a tab, for four spaces, and for a document printed compact
// alike. The line is then read as well, because an equality between two
// derived strings is hard to see a width in.
func TestJSONIsPrettyPrintedWithTwoSpaceIndentation(t *testing.T) {
	crHome(t)

	out := strings.TrimSuffix(throughAPipe(t, "config"), "\n")

	var compact, indented bytes.Buffer
	require.NoError(t, json.Compact(&compact, []byte(out)), "a pipe was given %q", out)
	require.NoError(t, json.Indent(&indented, compact.Bytes(), "", "  "))
	assert.Equal(t, indented.String(), out)
	assert.Contains(t, out, "\n  \"post.max_comments\": 20")
}

// outputDeciders are the imports that would let a file under internal/cli
// answer §12.1 for itself: the terminal check, the encoder, and the colour.
// Each is a way of deciding what output looks like, and the writer is where all
// three are already decided.
var outputDeciders = map[string]string{
	"github.com/mattn/go-isatty": "asks for itself whether stdout is a terminal",
	"encoding/json":              "encodes its own output",
	"github.com/fatih/color":     "reaches for colour of its own",
}

// §12.1's decision is made once, in the shared writer, and not per command.
//
// The rule is not that today's commands happen to route through it — they do,
// and a reader can check that by eye — but that a command added later cannot
// quietly reach a second answer. Imports are what that costs: a command cannot
// consult a terminal, encode a document, or emit a colour without naming the
// package that does it, and none of the three can be spelled around.
//
// Only internal/cli is fenced. A package below it parses JSON from files and
// must go on doing so; what it may not do is decide what a command prints,
// which it has no way to reach from there anyway.
func TestOnlyTheSharedWriterDecidesTheOutputShape(t *testing.T) {
	commands := filepath.Join("internal", "cli") + string(filepath.Separator)
	theWriter := filepath.Join("internal", "cli", "output.go")

	var found []string
	crSource(t, func(rel string, imports []string) {
		if !strings.HasPrefix(rel, commands) || rel == theWriter {
			return
		}
		for _, name := range imports {
			if why, ok := outputDeciders[name]; ok {
				found = append(found, rel+" "+why)
			}
		}
	})

	assert.Empty(t, found,
		"§12.1: the output shape is settled once in "+theWriter+", so no command may settle it again")
}

// Both shapes belong to every command, not to the one whose own test happens to
// read them. `cr config` proves the pair above; `cr init` is the other command
// in the tree, and its terminal rendering has a job of its own — §2.2 puts the
// state tree wherever $CR_HOME says, so the directory it names is the answer to
// a question the user could not have answered themselves.
func TestATerminalInitNamesTheStateDirectory(t *testing.T) {
	root := crHome(t)

	out := throughATerminal(t, "init")

	assert.Contains(t, out, "state directory ready at ")
	assert.Contains(t, out, "\x1b[36m"+root+"\x1b[0m")
}
