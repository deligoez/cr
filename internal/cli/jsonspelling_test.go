package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §12.1 on a failure, for the spellings of `--json` pflag reads: a failing
// command in a terminal reports its error as a document for every value
// strconv.ParseBool takes as true, and as prose when the last occurrence is
// false, so the failure's shape is the one a success would take under the same
// flag. The binary is run with stdout on a real pseudo-terminal because
// reportFailure reads the raw arguments and the terminal, and only the process
// joins the two the way Execute does.
//
// `--json=maybe --json` is a value pflag refuses: it stops there with a usage
// error before reaching the bare flag, so the run is prose.
func TestAFailureInATerminalReadsEveryJSONSpellingPflagReads(t *testing.T) {
	binary := crBinary(t)
	home := filepath.Join(t.TempDir(), ".cr")

	run := func(t *testing.T, flags ...string) string {
		t.Helper()
		master, terminal, err := pty.Open()
		require.NoError(t, err)
		t.Cleanup(func() { master.Close(); terminal.Close() })

		cmd := exec.Command(binary, append([]string{"status", "abc", "--repo", "o/r"}, flags...)...)
		cmd.Dir = t.TempDir()
		cmd.Env = append(cmd.Environ(), state.HomeEnv+"="+home)
		cmd.Stdout = terminal
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		var exited *exec.ExitError
		require.True(t, errors.As(cmd.Run(), &exited), "the run did not fail; stderr: %s", stderr.String())
		assert.Equal(t, ExitUsage, exited.ExitCode(), "stderr: %s", stderr.String())
		return stderr.String()
	}

	refusals := make(map[string]string)
	for _, row := range []struct {
		flags  []string
		asJSON bool
	}{
		{[]string{"--json=1"}, true},
		{[]string{"--json=TRUE"}, true},
		{[]string{"--json", "--json=false"}, false},
		{[]string{"--json=maybe", "--json"}, false},
	} {
		name := strings.Join(row.flags, " ")
		t.Run(name, func(t *testing.T) {
			stderr := run(t, row.flags...)
			var reported failure
			if row.asJSON {
				require.NoError(t, json.Unmarshal([]byte(stderr), &reported), "a terminal under %s was given %q", name, stderr)
			} else {
				lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
				require.Len(t, lines, 2, "a terminal under %s was given %q", name, stderr)
				var ok bool
				reported.Error, ok = strings.CutPrefix(lines[0], "error: ")
				require.True(t, ok, "the first line is the error: %q", lines[0])
				reported.Hint, ok = strings.CutPrefix(lines[1], "hint: ")
				require.True(t, ok, "the second line is the hint: %q", lines[1])
			}
			assert.Equal(t, usageHint, reported.Hint)
			refusals[name] = reported.Error
		})
	}
	assert.Equal(t, refusals["--json=1"], refusals["--json=TRUE"])
	assert.Equal(t, refusals["--json=1"], refusals["--json --json=false"])
}
