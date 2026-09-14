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
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
// error before reaching the bare flag, so the run is prose, and `--json
// --json=maybe` is prose too, because pflag stores the refused value's false
// before it stops. A `--json` that a string flag takes as its value — the
// root's `--repo`, or `cr test`'s own `--filter` — is that flag's value and
// asks for nothing, so the run is prose. It stays prose beside a flag cobra
// adds only when it executes: the help flag in both spellings, the root's
// version flag, and the flags of the completion command cobra registers.
//
// An unknown flag stops the parse where it stands, and the answer is what the
// flags before it set: `--json --bogus` is JSON, while `--bogus --json` never
// reaches `--json` and `--repo --json --bogus` gave it to `--repo`, so both
// are prose.
func TestAFailureInATerminalReadsEveryJSONSpellingPflagReads(t *testing.T) {
	binary := crBinary(t)
	home := filepath.Join(t.TempDir(), ".cr")

	run := func(t *testing.T, args ...string) string {
		t.Helper()
		master, terminal, err := pty.Open()
		require.NoError(t, err)
		t.Cleanup(func() { master.Close(); terminal.Close() })

		cmd := exec.Command(binary, args...)
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
		args   []string
		asJSON bool
	}{
		{[]string{"status", "abc", "--repo", "o/r", "--json=1"}, true},
		{[]string{"status", "abc", "--repo", "o/r", "--json=TRUE"}, true},
		{[]string{"status", "abc", "--repo", "o/r", "--json", "--json=false"}, false},
		{[]string{"status", "abc", "--repo", "o/r", "--json=maybe", "--json"}, false},
		{[]string{"status", "abc", "--repo", "o/r", "--json", "--json=maybe"}, false},
		{[]string{"status", "abc", "--repo", "--json", "--bogus"}, false},
		{[]string{"status", "abc", "--bogus", "--json"}, false},
		{[]string{"status", "abc", "--json", "--bogus"}, true},
		{[]string{"status", "abc", "--repo", "--json"}, false},
		{[]string{"test", "abc", "--repo", "o/r", "--filter", "--json"}, false},
		{[]string{"status", "abc", "--repo", "--json", "--help=false"}, false},
		{[]string{"status", "abc", "--repo", "--json", "-h=false"}, false},
		{[]string{"x", "--repo", "--json", "--version=false"}, false},
		{[]string{"completion", "bash", "x", "--no-descriptions", "--repo", "--json"}, false},
	} {
		name := strings.Join(row.args, " ")
		t.Run(name, func(t *testing.T) {
			stderr := run(t, row.args...)
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
	pr := "status abc --repo o/r --json=1"
	for _, name := range []string{
		"status abc --repo o/r --json=TRUE",
		"status abc --repo o/r --json --json=false",
		"status abc --repo --json",
		"test abc --repo o/r --filter --json",
		"status abc --repo --json --help=false",
		"status abc --repo --json -h=false",
	} {
		assert.Equal(t, refusals[pr], refusals[name], name)
	}
}

// §12.1's failure path over the whole tree cobra executes. asksForJSON parses
// a fresh tree, and every command the run's own ExecuteC leaves in the tree —
// the help and completion commands it registers included — must read `--json`
// there as it read it when executed: on `<command> --repo --json`, where the
// string flag takes `--json` as its value, and on that line with each boolean
// flag the executed command defines set to false, which reaches the help,
// version and completion flags cobra adds only at execute time.
//
// It must also read it as the run did where the run's parse refused the line:
// an unknown flag after `--repo --json`, after `--json`, and before `--json`,
// where pflag's flag set holds what it had set when it stopped. An unknown
// command is swept as a path of its own, because the run parses its flags only
// while the root's Args leaves cobra's lookup nothing to refuse.
//
// The executed side is a real in-process run; crHome, a temporary working
// directory and TestMain's gh fence keep what the commands do to themselves.
func TestTheFailurePathReadsJSONAsTheExecutedCommandDoes(t *testing.T) {
	crHome(t)
	t.Chdir(t.TempDir())

	execute := func(args []string) *cobra.Command {
		root := newRootCmd()
		root.SetOut(&discard{})
		root.SetErr(&discard{})
		root.SetArgs(args)
		cmd, _ := root.ExecuteC()
		return cmd
	}

	paths := make([][]string, 0)
	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		paths = append(paths, path)
		for _, sub := range cmd.Commands() {
			walk(sub, append(append([]string{}, path...), sub.Name()))
		}
	}
	walk(execute([]string{"--help"}).Root(), []string{})
	paths = append(paths, []string{"x"})

	flagged := make(map[string]bool)
	checked := 0
	for _, path := range paths {
		base := append(append([]string{}, path...), "--repo", "--json")
		lines := [][]string{
			base,
			append(append([]string{}, base...), "--bogus"),
			append(append([]string{}, path...), "--json", "--bogus"),
			append(append([]string{}, path...), "--bogus", "--json"),
		}
		execute(base).Flags().VisitAll(func(f *pflag.Flag) {
			if f.Value.Type() != "bool" || f.Name == "json" {
				return
			}
			flagged[f.Name] = true
			lines = append(lines, append(append([]string{}, base...), "--"+f.Name+"=false"))
			if f.Shorthand != "" {
				lines = append(lines, append(append([]string{}, base...), "-"+f.Shorthand+"=false"))
			}
		})
		for _, line := range lines {
			name := strings.Join(line, " ")
			executed := execute(line)
			require.True(t, executed.Flags().Parsed(), "the run did not parse `cr %s`", name)
			asked, err := executed.Flags().GetBool("json")
			require.NoError(t, err)
			assert.Equal(t, asked, asksForJSON(line), "`cr %s`", name)
			checked++
		}
	}
	for _, name := range []string{"help", "version", "no-descriptions"} {
		assert.True(t, flagged[name], "no executed command defined --%s, so the sweep never reached it", name)
	}
	require.Greater(t, checked, 3*len(paths), "the sweep checked too few lines to prove anything")
}
