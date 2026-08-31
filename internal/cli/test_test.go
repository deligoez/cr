package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §5.2.1 through the command: `cr test` runs the profile's test command inside
// the sandbox, narrowed by `tests.filter_flag`, and reports §5.1.6's recreation.
//
// No sandbox is created beforehand, and that is deliberate. §5.1.6 has cr verify
// the sandbox before every run and recreate one that fails the check, and an
// absent sandbox fails it — so this exercises the check, the recreation, and the
// notice in one go. The notice is asserted in the payload because §11.1 exempts
// it from `--quiet`: a sandbox rebuilt in silence is a run whose previous probe
// left something behind, read by the author as if nothing had happened.
//
// The runner writes its own working directory and its own arguments to a log
// outside the sandbox, so what is checked is where the command stood and what it
// was given, rather than what the payload says about itself.
func TestTheTestCommandRunsTheProfileRunnerInTheSandbox(t *testing.T) {
	fixture := fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	scripts := t.TempDir()
	log := filepath.Join(scripts, "observed.log")
	runner := filepath.Join(scripts, "runner.sh")
	require.NoError(t, os.WriteFile(runner,
		[]byte("#!/bin/sh\n{ pwd; echo \"$@\"; } > "+log+"\n"), 0o700))

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"tests":{"cmd":["`+runner+`"],"globs":["*_test.txt"],"filter_flag":"--only"}}`))
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, ProfileID: "qa", Round: 1, Head: head,
	}))
	require.NoError(t, held.Unlock())

	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })

	var printed map[string]any
	require.NoError(t, json.Unmarshal([]byte(throughAPipe(t,
		"test", fixturePR, "--repo", fixtureSlug, "--filter", "retries twice")), &printed))

	sandboxPath := prepared.Sandbox(fixtureOwner, fixtureProject, fixturePRNumber)
	assert.Equal(t, sandboxPath, printed["sandbox"])
	assert.Equal(t, []any{runner, "--only", "retries twice"}, printed["command"],
		"§5.2.1: --filter is passed as the profile's tests.filter_flag")
	assert.Equal(t, "retries twice", printed["filter"])
	assert.InDelta(t, 0, printed["exit_code"], 0)

	honesty, ok := printed["honesty"].([]any)
	require.True(t, ok, "§11.1's disclosures are a field on the payload")
	require.Len(t, honesty, 1, "§5.1.6: the sandbox was absent, so it was recreated and said so")
	assert.Contains(t, honesty[0], "§5.1.6")
	assert.Contains(t, honesty[0], sandboxPath)

	observed, err := os.ReadFile(log)
	require.NoError(t, err, "the profile's test command never ran")
	// The sandbox path is resolved through its symlinks, because a
	// temporary directory reaches `pwd` as the path the kernel resolved.
	resolved, err := filepath.EvalSymlinks(sandboxPath)
	require.NoError(t, err)
	ran := strings.Split(strings.TrimSpace(string(observed)), "\n")
	require.Len(t, ran, 2)
	assert.Equal(t, resolved, ran[0], "§5.2.1: the suite runs inside the sandbox")
	assert.Equal(t, "--only retries twice", ran[1])
}

// §5.2.4 through the command: every run is stored in runs.ndjson under an id
// of the form r<n>, and the id space is what a probe's `baseline` field
// references — so the command prints the id it allocated and the second run of
// the same pull request gets the next one rather than overwriting the first.
//
// The runner exits non-zero and prints more than the profile retains, which is
// what makes the record worth asserting on: the exit code reaches the file
// unread, `timed_out` says the run finished rather than being killed, the tail
// is the end of the output and not the beginning, and the two counts are
// absent because no `tests.count_pattern` is configured to derive them.
func TestTheTestCommandStoresARunRecordForEveryRun(t *testing.T) {
	fixture := fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	printed := "line one\nline two\nTests:  4 passed\n"
	runner := filepath.Join(t.TempDir(), "runner.sh")
	require.NoError(t, os.WriteFile(runner, []byte(
		"#!/bin/sh\necho 'line one'\necho 'line two' >&2\necho 'Tests:  4 passed'\nexit 3\n"), 0o700))

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"tests":{"cmd":["`+runner+`"],"globs":["*_test.txt"],"output_tail_bytes":16}}`))
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, ProfileID: "qa", Round: 3, Head: head,
	}))
	require.NoError(t, held.Unlock())

	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })

	for i, want := range []string{"r1", "r2"} {
		// The runner's own output reaches the reader as it is
		// produced, ahead of the document §12.1 keeps stdout for, so
		// the two are separated here rather than the runner being
		// silenced: what is stored has to be what was shown.
		shown := throughAPipe(t, "test", fixturePR, "--repo", fixtureSlug)
		require.True(t, strings.HasPrefix(shown, printed),
			"run %d: the runner's output reaches the reader as it is produced", i+1)

		var reported map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(shown, printed)), &reported))
		assert.Equal(t, want, reported["run"], "run %d reports the id it allocated", i+1)
	}

	body, err := os.ReadFile(prepared.PRFile(
		fixtureOwner, fixtureProject, fixturePRNumber, state.FileRuns))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	require.Len(t, lines, 2, "§5.2.4: a run is added to runs.ndjson, never written over it")

	var stored map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &stored))
	duration, ok := stored["duration_ms"].(float64)
	require.True(t, ok, "§5.2.4 stores the duration")
	assert.GreaterOrEqual(t, duration, float64(0))
	delete(stored, "duration_ms")
	assert.Equal(t, map[string]any{
		"id":          "r1",
		"head":        head,
		"round":       float64(3),
		"exit_code":   float64(3),
		"timed_out":   false,
		"output_tail": printed[len(printed)-16:],
		"passed":      false,
	}, stored,
		"§5.2.4: no filter was given, no count is derivable, and the run was not a probe's")
}
