package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/sandbox"
	"github.com/deligoez/cr/internal/state"
)

// §5.2.1 through `cr test`: the runner is started without the variables
// coding-agent detectors read, so a runner that prints differently under an
// agent prints the recap tests.count_pattern reads.
//
// Measured on tarfin-labs/backend#6328 with cr 0.13.0: under Claude Code
// (AI_AGENT=claude-code_…) Pest 4 printed one JSON line in place of its recap,
// so rounds 1 and 2 read tests_run null and passed false over an exit of 0,
// and round 3, with AI_AGENT unset, read 21 run, 0 failed, and passed. The
// runner here behaves the same way for every variable on the list.
func TestTheRunnerDoesNotSeeACodingAgent(t *testing.T) {
	for _, name := range sandbox.AgentVariables {
		t.Setenv(name, "claude-code_test")
	}
	countedTestHome(t, "#!/bin/sh\n"+
		"if env | grep -q -E '^("+strings.Join(sandbox.AgentVariables, "|")+")='; then\n"+
		"  echo '{\"tool\":\"pest\",\"result\":\"passed\",\"tests\":21,\"passed\":21}'\n"+
		"else\n"+
		"  echo 'Tests:  21 passed'\n"+
		"fi\n")

	printed := printedTestRun(t)
	assert.Equal(t, []any{float64(21), float64(0), true},
		[]any{printed["tests_run"], printed["tests_failed"], printed["passed"]})
}

// countedTestHome is a round whose profile runs script as its test command
// and reads `Tests:  <n> passed` and `Tests:  <n> failed` as its counts.
func countedTestHome(t *testing.T, script string) {
	t.Helper()
	fixture := fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))
	runner := filepath.Join(t.TempDir(), "runner.sh")
	require.NoError(t, os.WriteFile(runner, []byte(script), 0o700))

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"tests":{"cmd":["`+runner+`"],"globs":["*_test.txt"],`+
		`"count_pattern":"Tests:\\s+(\\d+)\\s+(?:failed|passed)\\b",`+
		`"failed_pattern":"Tests:\\s+(\\d+)\\s+failed\\b"}}`))
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
}

// printedTestRun is the document `cr test` printed to stdout.
func printedTestRun(t *testing.T, flags ...string) map[string]any {
	t.Helper()
	stdout, _ := streams(t, append([]string{"test", fixturePR, "--repo", fixtureSlug}, flags...)...)
	var printed map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &printed))
	return printed
}
