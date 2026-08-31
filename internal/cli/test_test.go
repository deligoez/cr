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
