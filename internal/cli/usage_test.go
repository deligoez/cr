package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// usageRuns are malformed invocations of commands taking different argument
// shapes, each with the fault §11.2's code 2 is for: a missing required
// argument, a missing required flag, an unknown flag, a flag missing its value,
// an unparseable pull request reference, and a command that does not exist.
//
// Every one of them is wrong before any file is opened or any state is read, so
// the run never reaches the gh fence TestMain installs either.
var usageRuns = map[string][]string{
	"record missing its file":          {"record", "1"},
	"record with an unknown flag":      {"record", "1", "merged.ndjson", "--repo", "o/r", "--bogus"},
	"record with an unparseable pr":    {"record", "abc", "merged.ndjson", "--repo", "o/r"},
	"note missing both arguments":      {"note"},
	"status with an unparseable pr":    {"status", "abc", "--repo", "o/r"},
	"merge missing its required flags": {"merge", "a.ndjson", "--repo", "o/r"},
	"merge with an unparseable --pr":   {"merge", "a.ndjson", "-o", "out", "--repo", "o/r", "--pr", "abc"},
	"probe run with --kind unvalued":   {"probe", "run", "1", "--repo", "o/r", "--kind"},
	"a command that does not exist":    {"frobnicate"},
}

// §11.2: a malformed invocation exits 2, and only through the process's own
// exit status is that observable — cobra returns its usage errors as values and
// leaves the status to whoever calls it, so a test of exitCodeFor alone would
// not see a caller that forgot to map them. The binary is built and run, and
// every run carries §12.4's step as well.
func TestEveryUsageErrorExitsTwoFromTheProcess(t *testing.T) {
	binary := crBinary(t)
	home := filepath.Join(t.TempDir(), ".cr")

	for name, args := range usageRuns {
		t.Run(name, func(t *testing.T) {
			run := exec.Command(binary, args...)
			run.Dir = t.TempDir()
			run.Env = append(run.Environ(), state.HomeEnv+"="+home)
			var stderr bytes.Buffer
			run.Stderr = &stderr

			err := run.Run()

			var exited *exec.ExitError
			require.True(t, errors.As(err, &exited), "the run did not fail: %v", err)
			assert.Equal(t, ExitUsage, exited.ExitCode(), "stderr: %s", stderr.String())
			// stdout is not a terminal here, so §12.1 makes the failure a
			// document; it is decoded rather than matched, because the
			// encoder escapes the hint's angle brackets.
			var reported failure
			require.NoError(t, json.Unmarshal(stderr.Bytes(), &reported), "stderr: %s", stderr.String())
			assert.Equal(t, usageHint, reported.Hint)
		})
	}
}

// The same invocations, one level down: no row of the exit code table claims a
// usage error, so each falls through to ExitUsage by the absence of a mapping
// rather than by one written for it. A row that started claiming one would turn
// a mistyped command line into a file or state failure.
func TestNoRowClaimsAUsageError(t *testing.T) {
	crHome(t)
	for name, args := range usageRuns {
		t.Run(name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&discard{})
			cmd.SetErr(&discard{})
			cmd.SetArgs(args)

			err := cmd.Execute()

			require.Error(t, err)
			assert.Nil(t, rowFor(err), "a row claims %v", err)
			assert.Equal(t, ExitUsage, exitCodeFor(err))
		})
	}
}
