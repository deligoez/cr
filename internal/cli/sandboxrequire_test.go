package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// requireFixture is envFixture with a profile whose `sandbox.require` names
// required and whose `sandbox.copy` names copied, so a test can put the two
// lists into disagreement.
//
// The runner is the env reporter of the v0.2.2 QA repro, which says which
// environment file a Laravel-shaped suite would have read — the outcome §5.1.8
// exists to prevent is that suite running against the wrong one, and a run cr
// refused prints nothing at all.
func requireFixture(
	t *testing.T, copied, required []string,
) (prepared state.Layout, sandboxPath, runner, profileFile string) {
	t.Helper()
	_, sandboxPath, runner, profileFile = envFixture(t, copied, ".env", ".env.testing")
	encodedCopy, err := json.Marshal(copied)
	require.NoError(t, err)
	encodedRequire, err := json.Marshal(required)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(profileFile, []byte(`{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"sandbox":{"copy":`+string(encodedCopy)+`,"require":`+string(encodedRequire)+`},`+
		`"tests":{"cmd":["`+runner+`"],"globs":["*_test.txt"],"filter_flag":"--only",`+
		`"count_pattern":"Tests:  ([0-9]+) (?:failed|passed)","failed_pattern":"Tests:  ([0-9]+) failed"}}`), 0o600))
	require.NoError(t, os.WriteFile(runner, []byte(envReporter), 0o700))
	return state.New(crHomeOf(t)), sandboxPath, runner, profileFile
}

// §5.1.8: "After §5.1.6's check and before any run, `cr test` and `cr probe run`
// MUST abort with exit code 3 when a `sandbox.require` path does not exist in
// the sandbox, naming the path and `sandbox.copy`."
//
// Both commands are driven, because the section binds both and they reach the
// check by different routes — `cr test` through its own §5.1.6 call, a probe
// through the one that also precedes §5.2.6's baseline.
//
// What is asserted beside the code is that nothing ran. A suite started without
// the file it needs does not usually stop: it falls back, finishes, and reports
// a result nobody can tell from a real one, which is the whole reason the field
// is binding rather than advisory (spec/field-feedback.md, 2.1).
func TestBothRunningCommandsRefuseASandboxMissingARequiredPath(t *testing.T) {
	for name, args := range map[string][]string{
		"cr test": {"test", fixturePR, "--repo", fixtureSlug},
		"cr probe run": {"probe", "run", fixturePR, "--repo", fixtureSlug,
			"--kind", "mutation"},
	} {
		t.Run(name, func(t *testing.T) {
			prepared, sandboxPath, _, profileFile := requireFixture(t,
				[]string{".env"}, []string{".env.testing"})
			if name == "cr probe run" {
				args = append(args, "--patch", writePatch(t, fixtureDiff))
			}

			err := runCLI(t, args...)

			require.Error(t, err)
			assert.Equal(t, ExitFile, exitCodeFor(err), "§5.1.8: §11.2's 3")
			assert.Contains(t, err.Error(), filepath.Join(sandboxPath, ".env.testing"),
				"§5.1.8: the refusal names the path")
			assert.Contains(t, hintFor(err), "sandbox.copy in "+profileFile,
				"§5.1.8: and the field that would put it there")
			assert.Empty(t, storedRecords(t, prepared, state.FileRuns),
				"the refusal lands before any run, so nothing was recorded")
		})
	}
}

// The other direction: a `sandbox.require` entry §5.1.2 copied in is held by
// the sandbox, so the run proceeds and the suite reads the file.
//
// It is asserted through the runner's own output rather than through the exit
// code. A command that skipped the check entirely would pass an exit-code
// assertion here and fail nothing, and the fact worth pinning is that the
// admitted run is the one that reads `.env.testing`.
func TestARequiredPathTheSandboxHoldsAdmitsTheRun(t *testing.T) {
	prepared, sandboxPath, _, _ := requireFixture(t,
		[]string{".env", ".env.testing"}, []string{".env.testing"})

	stdout, stderr := streams(t, "test", fixturePR, "--repo", fixtureSlug)

	require.FileExists(t, filepath.Join(sandboxPath, ".env.testing"))
	assert.Contains(t, afterHeader(t, stderr), "env .env.testing",
		"the suite ran against the file sandbox.require names")
	assert.Equal(t, "r1", document(t, stdout)["run"])
	assert.Len(t, storedRecords(t, prepared, state.FileRuns), 1)
}

// A `sandbox.require` entry that is not a path inside the repository is the
// malformed profile field §2.5 item 3 codes 3, naming the file and the field —
// the same refusal `sandbox.copy` already gets, and for the same reason: an
// absolute entry would have cr look outside the sandbox and answer about a file
// no run could ever have used.
func TestARequiredPathOutsideTheRepositoryIsMalformed(t *testing.T) {
	for name, entry := range map[string]string{
		"an absolute entry": "/etc/passwd",
		"an escaping entry": "../elsewhere/.env",
		"an empty entry":    "",
	} {
		t.Run(name, func(t *testing.T) {
			_, _, _, profileFile := requireFixture(t, []string{".env"}, []string{entry})

			err := runCLI(t, "test", fixturePR, "--repo", fixtureSlug)

			require.Error(t, err)
			assert.Equal(t, ExitFile, exitCodeFor(err), "§2.5 item 3: a malformed profile field exits 3")
			assert.Contains(t, err.Error(), "sandbox.require")
			assert.Contains(t, err.Error(), profileFile)
		})
	}
}
