package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/creack/pty"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §12.4 through a pipe: a failed run's error is a JSON document whose `hint`
// field names the next step, and `--json` asks for the same document whatever
// stdout is.
func TestAFailureThroughAPipeIsADocumentCarryingAHint(t *testing.T) {
	_, pipe, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { pipe.Close() })
	missing := state.FileFailure("read", "/x/mapping.ndjson", "run `cr map record`", os.ErrNotExist)

	for name, args := range map[string][]string{
		"piped":        {"record", "1", "merged.ndjson"},
		"asked --json": {"record", "1", "merged.ndjson", "--json"},
	} {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			require.NoError(t, reportFailure(pipe, &stderr, args, missing))

			var reported map[string]string
			require.NoError(t, json.Unmarshal(stderr.Bytes(), &reported), "stderr: %s", stderr.String())
			assert.Equal(t, missing.Error(), reported["error"])
			assert.Equal(t, "run `cr map record`", reported["hint"])
			assert.Contains(t, stderr.String(), "\n  \"hint\"", "§12.2's two-space indentation")
		})
	}
}

// §12.1's other shape: in a terminal the same failure is two prose lines, and
// `--json` still turns it into the document.
func TestAFailureInATerminalIsProseUnlessJSONIsAsked(t *testing.T) {
	master, terminal, err := pty.Open()
	require.NoError(t, err)
	t.Cleanup(func() { master.Close(); terminal.Close() })
	refused := errors.New("unknown flag: --bogus")

	var prose bytes.Buffer
	require.NoError(t, reportFailure(terminal, &prose, []string{"record", "--bogus"}, refused))
	assert.Equal(t, "error: unknown flag: --bogus\nhint: "+usageHint+"\n", prose.String())

	var document bytes.Buffer
	require.NoError(t, reportFailure(terminal, &document, []string{"record", "--json", "--bogus"}, refused))
	assert.True(t, json.Valid(document.Bytes()), "stderr: %s", document.String())

	var afterDashes bytes.Buffer
	require.NoError(t, reportFailure(terminal, &afterDashes, []string{"note", "--", "--json"}, refused))
	assert.False(t, json.Valid(afterDashes.Bytes()), "a --json after -- is an argument, not the flag")
}

// Every row of §11.2's table names a step. init refuses an empty one at package
// load; this says so in a test's words, so the rule is visible where rules are
// read and not only where the package panics.
func TestEveryRowOfTheExitTableNamesAStep(t *testing.T) {
	require.NotEmpty(t, codes)
	for i := range codes {
		assert.NotEmpty(t, strings.TrimSpace(codes[i].hint), "row %d", i)
	}
	assert.NotEmpty(t, hintFor(&state.FileError{}),
		"a file failure built without FileFailure still takes the floor's step")
}

// §12.4 over the whole command tree: every leaf command is run the ways a
// caller gets wrong most often — no arguments, too many, and an unknown flag —
// and every error any of them returns carries a non-empty hint.
//
// It walks the tree rather than a list, so a command added later is reached the
// day it is registered. hintFor is total by construction (a claimed row, the
// file's own step, or the usage hint), and this is what holds it to that over
// errors nobody wrote a row for.
func TestEveryErrorTheCommandTreeReturnsCarriesAHint(t *testing.T) {
	crHome(t)
	leaves := make([][]string, 0)
	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		if !cmd.HasSubCommands() {
			leaves = append(leaves, path)
			return
		}
		for _, sub := range cmd.Commands() {
			if sub.Name() == "help" || sub.Name() == "completion" {
				continue
			}
			walk(sub, append(append([]string{}, path...), sub.Name()))
		}
	}
	walk(newRootCmd(), nil)
	require.Greater(t, len(leaves), 20, "the walk reached too few commands to prove anything")

	failures := 0
	for _, leaf := range leaves {
		for _, extra := range [][]string{{}, {"x", "y", "z", "w"}, {"--no-such-flag"}} {
			cmd := newRootCmd()
			cmd.SetOut(&discard{})
			cmd.SetErr(&discard{})
			cmd.SetArgs(append(append([]string{}, leaf...), extra...))
			err := cmd.Execute()
			if err == nil {
				continue
			}
			failures++
			assert.NotEmpty(t, strings.TrimSpace(hintFor(err)), "`cr %s` %v: %v", strings.Join(leaf, " "), extra, err)
		}
	}
	require.Greater(t, failures, len(leaves), "most runs should have failed; the sweep measured little")
}
