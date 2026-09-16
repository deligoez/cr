package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// afterHeader returns what a `cr test` or `cr probe run` printed after the
// experiment header that opens it, and fails when there is no header.
func afterHeader(t *testing.T, shown string) string {
	t.Helper()
	require.True(t, strings.HasPrefix(shown, "experiment "), "the experiment header opens the output: %q", shown)
	lines := strings.SplitAfter(shown, "\n")
	rest := 1
	for rest < len(lines) && strings.HasPrefix(lines[rest], "  ") {
		rest++
	}
	return strings.Join(lines[rest:], "")
}

// envFixture is probeFixture's pull request with the clone root holding the
// gitignored files ignored and a profile whose `sandbox.copy` is copied. The
// files are ignored through .git/info/exclude, which `--exclude-standard`
// reads, so no commit of the fixture changes. It returns the clone root as git
// resolves it, the sandbox path, the runner, and the profile file.
func envFixture(
	t *testing.T, copied []string, ignored ...string,
) (root, sandboxPath, runner, profileFile string) {
	t.Helper()
	fixture := fixtureRepository(t)
	home := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	info := filepath.Join(fixture, ".git", "info")
	require.NoError(t, os.MkdirAll(info, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(info, "exclude"),
		[]byte(strings.Join(ignored, "\n")+"\n"), 0o600))
	for _, name := range ignored {
		require.NoError(t, os.WriteFile(filepath.Join(fixture, name), []byte("SECRET=never-read\n"), 0o600))
	}

	runner = filepath.Join(t.TempDir(), "runner.sh")
	require.NoError(t, os.WriteFile(runner, []byte("#!/bin/sh\necho 'Tests:  4 passed'\n"), 0o700))
	encodedCopy, err := json.Marshal(copied)
	require.NoError(t, err)

	prepared := state.New(home)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"sandbox":{"copy":`+string(encodedCopy)+`},`+
		`"tests":{"cmd":["`+runner+`"],"globs":["*_test.txt"],"filter_flag":"--only",`+
		`"count_pattern":"Tests:  ([0-9]+) (?:failed|passed)","failed_pattern":"Tests:  ([0-9]+) failed"}}`))
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, ProfileID: "qa", Round: 2, Head: head,
	}))
	require.NoError(t, held.Unlock())

	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })
	base := strings.TrimSpace(mustGit(t, fixture, "rev-parse", "main"))
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	root = strings.TrimSpace(mustGit(t, fixture, "rev-parse", "--show-toplevel"))
	return root, prepared.Sandbox(fixtureOwner, fixtureProject, fixturePRNumber),
		runner, prepared.Profile("qa")
}

// streams runs one command with standard output and standard error apart, so
// the header is asserted on the stream it is written to and the document on
// the other.
func streams(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	var out, errs bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errs)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute(), "stderr: %s", errs.String())
	return out.String(), errs.String()
}

// documentKeys decodes stdout as one JSON document and returns its keys.
func documentKeys(t *testing.T, stdout string) []string {
	t.Helper()
	var document map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &document), "standard output is one whole document: %q", stdout)
	keys := make([]string, 0, len(document))
	for key := range document {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

const recapLine = "Tests:  4 passed\n"

// Both gitignored env files copied: `cr test` names the runner, the sandbox,
// the recreation that built it, both files and that the sandbox holds both,
// says nothing is missing, and leaves the document on standard output as it
// was.
func TestTheTestHeaderListsTheCopiedEnvFiles(t *testing.T) {
	_, sandboxPath, runner, _ := envFixture(t, []string{".env", ".env.testing"}, ".env", ".env.testing")

	stdout, stderr := streams(t, "test", fixturePR, "--repo", fixtureSlug)

	opening := "experiment " + runner + "\n" +
		"  sandbox    " + sandboxPath + "\n"
	envLines := "  env files  .env, .env.testing gitignored at the clone root\n" +
		"  in sandbox .env, .env.testing\n"
	assert.Equal(t, opening+"  recreated  per §5.1.6: there is no sandbox at that path\n"+envLines+recapLine, stderr)
	assert.Equal(t, []string{"command", "exit_code", "honesty", "run", "sandbox", "timed_out", "warnings"},
		documentKeys(t, stdout))

	_, quiet := streams(t, "test", fixturePR, "--repo", fixtureSlug, "--quiet")
	assert.Equal(t, opening+envLines, quiet, "§11.1: --quiet takes the runner's echo and leaves the header")
}

// A gitignored `.env.local` no `sandbox.copy` entry names: the header says the
// suite runs without it and names the field, and a filtered probe with no
// baseline at the head says which run comes first. The second probe finds that
// baseline recorded and says nothing about one.
//
// §5.2.2 makes a mutation probe's baseline "the run with the probe's filter and
// paths", so the run announced here carries the same `--only retries` the probe
// does: one baseline, not the whole suite beside it.
func TestTheProbeHeaderNamesTheUncopiedFileAndTheImplicitBaseline(t *testing.T) {
	root, sandboxPath, runner, profileFile := envFixture(t,
		[]string{".env", ".env.testing"}, ".env", ".env.local", ".env.testing")
	patch := writePatch(t, fixtureDiff)
	probing := []string{"probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", patch, "--filter", "retries"}
	uncopied := ".env.local is gitignored at the clone root " + root +
		" and absent from the sandbox, so the suite runs without it; add it to sandbox.copy in " +
		profileFile + " to copy it in"
	opening := "experiment " + runner + " --only retries\n" +
		"  sandbox    " + sandboxPath + "\n"
	envLines := "  env files  .env, .env.local, .env.testing gitignored at the clone root\n" +
		"  in sandbox .env, .env.testing\n" +
		"  not copied " + uncopied + "\n"

	stdout, stderr := streams(t, probing...)
	assert.Equal(t, opening+"  recreated  per §5.1.6: there is no sandbox at that path\n"+envLines+
		"  baseline   §5.2.2's baseline is not on file and runs first: "+runner+" --only retries\n"+
		recapLine+recapLine, stderr,
		"§5.2.2: the baseline the filter selects, then the probe")
	assert.Equal(t, []string{"baseline", "command", "establishes", "filter", "honesty", "kind",
		"probe", "result", "run", "sandbox", "target", "warnings"}, documentKeys(t, stdout))
	assert.Equal(t, []any{"sandbox " + sandboxPath + " recreated, per §5.1.6: there is no sandbox at that path",
		uncopied}, honestyList(t, stdout), "the document carries the header's uncopied file too")

	stdout, again := streams(t, probing...)
	assert.Equal(t, opening+envLines+recapLine, again, "the baseline stands at the head, so only the probe runs")
	assert.Equal(t, []any{uncopied}, honestyList(t, stdout))

	stdout, _ = streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, []any{uncopied}, honestyList(t, stdout), "`cr test`'s document carries it as well")
}

// `cr sandbox create` reports the same uncopied file under honesty, and an
// empty list when every gitignored env file was copied.
func TestTheSandboxCreationDisclosesUncopiedEnvFiles(t *testing.T) {
	t.Run("uncopied", func(t *testing.T) {
		root, _, _, profileFile := envFixture(t, []string{".env"}, ".env", ".env.local")
		stdout, _ := streams(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
		var created map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &created))
		assert.Equal(t, []any{".env.local is gitignored at the clone root " + root +
			" and absent from the sandbox, so the suite runs without it; add it to sandbox.copy in " +
			profileFile + " to copy it in"}, created["honesty"])
	})
	t.Run("copied", func(t *testing.T) {
		envFixture(t, []string{".env", ".env.testing"}, ".env", ".env.testing")
		stdout, _ := streams(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
		var created map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &created))
		assert.Equal(t, []any{}, created["honesty"])
	})
}
