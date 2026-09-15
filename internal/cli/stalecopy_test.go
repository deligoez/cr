package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envReporter is a runner that says which env file a Laravel suite would read,
// the way the QA stub of v0.2.2's D-S22-1 did, and then passes.
const envReporter = "#!/bin/sh\n" +
	"if [ -f .env.testing ]; then echo 'env .env.testing'; else echo 'env .env'; fi\n" +
	"echo 'Tests:  4 passed'\n"

// rewriteProfile replaces the qa profile envFixture wrote with one copying
// copied and running setup, and makes runner the envReporter.
func rewriteProfile(t *testing.T, profileFile, runner string, copied, setup []string) {
	t.Helper()
	encodedCopy, err := json.Marshal(copied)
	require.NoError(t, err)
	encodedSetup, err := json.Marshal(setup)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(profileFile, []byte(`{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"sandbox":{"copy":`+string(encodedCopy)+`,"setup":`+string(encodedSetup)+`},`+
		`"tests":{"cmd":["`+runner+`"],"globs":["*_test.txt"],"filter_flag":"--only",`+
		`"count_pattern":"Tests:  ([0-9]+) (?:failed|passed)","failed_pattern":"Tests:  ([0-9]+) failed"}}`), 0o600))
	require.NoError(t, os.WriteFile(runner, []byte(envReporter), 0o700))
}

// honestyList decodes a command's document and returns its honesty list.
func honestyList(t *testing.T, stdout string) []any {
	t.Helper()
	var document map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &document), "standard output is one document: %q", stdout)
	honesty, ok := document["honesty"].([]any)
	require.True(t, ok, "the document carries a honesty list: %q", stdout)
	return honesty
}

// checkout is the directory envFixture made the repository under review.
func checkout(t *testing.T) string {
	t.Helper()
	dir, err := repoDir()
	require.NoError(t, err)
	return dir
}

// D-S22-1, the QA repro: a sandbox created while `sandbox.copy` did not name
// `.env.testing`, and a profile that now does. `cr test` recreates the sandbox
// before the run, says so in the header beside the argv and under honesty, and
// the runner reads `.env.testing`. The next run finds the sandbox holding every
// copied file and recreates nothing.
func TestATestRunRecreatesASandboxMissingACopiedFile(t *testing.T) {
	_, sandboxPath, runner, profileFile := envFixture(t, []string{".env"}, ".env", ".env.testing")
	rewriteProfile(t, profileFile, runner, []string{".env"}, []string{})
	streams(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
	require.NoFileExists(t, filepath.Join(sandboxPath, ".env.testing"))
	rewriteProfile(t, profileFile, runner, []string{".env", ".env.testing"}, []string{})

	reason := "sandbox.copy in " + profileFile + " names .env.testing, which the checkout " +
		checkout(t) + " holds and the sandbox does not"
	stdout, stderr := streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, "experiment "+runner+"\n"+
		"  sandbox    "+sandboxPath+"\n"+
		"  recreated  per §5.1.6: "+reason+"\n"+
		"  env files  .env, .env.testing gitignored at the clone root\n"+
		"  in sandbox .env, .env.testing\n"+
		"env .env.testing\n"+recapLine, stderr)
	assert.Equal(t, []any{"sandbox " + sandboxPath + " recreated, per §5.1.6: " + reason}, honestyList(t, stdout))

	stdout, stderr = streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, "experiment "+runner+"\n"+
		"  sandbox    "+sandboxPath+"\n"+
		"  env files  .env, .env.testing gitignored at the clone root\n"+
		"  in sandbox .env, .env.testing\n"+
		"env .env.testing\n"+recapLine, stderr, "a sandbox holding every copied file is not recreated")
	assert.Equal(t, []any{}, honestyList(t, stdout))
}

// D-S22-1 through `cr probe run`: the same stale sandbox is recreated before
// §5.2.6's baselines, and every run of the probe reads `.env.testing`.
func TestAProbeRunRecreatesASandboxMissingACopiedFile(t *testing.T) {
	_, sandboxPath, runner, profileFile := envFixture(t, []string{".env"}, ".env", ".env.testing")
	rewriteProfile(t, profileFile, runner, []string{".env"}, []string{})
	streams(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
	rewriteProfile(t, profileFile, runner, []string{".env", ".env.testing"}, []string{})

	reason := "sandbox.copy in " + profileFile + " names .env.testing, which the checkout " +
		checkout(t) + " holds and the sandbox does not"
	stdout, stderr := streams(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", writePatch(t, fixtureDiff), "--filter", "retries")
	assert.Equal(t, "experiment "+runner+" --only retries\n"+
		"  sandbox    "+sandboxPath+"\n"+
		"  recreated  per §5.1.6: "+reason+"\n"+
		"  env files  .env, .env.testing gitignored at the clone root\n"+
		"  in sandbox .env, .env.testing\n"+
		"  baseline   the whole suite runs first as the §5.2.2 baseline: "+runner+"\n"+
		strings.Repeat("env .env.testing\n"+recapLine, 3), stderr)
	assert.Equal(t, []any{"sandbox " + sandboxPath + " recreated, per §5.1.6: " + reason}, honestyList(t, stdout))
}

// Presence is not equivalence: `.env.testing` edited in the checkout after the
// sandbox copied it, to bytes of the same length, makes the sandbox stale.
func TestATestRunRecreatesASandboxWhoseCopiedFileDiffers(t *testing.T) {
	_, sandboxPath, runner, profileFile := envFixture(t, []string{".env", ".env.testing"}, ".env", ".env.testing")
	rewriteProfile(t, profileFile, runner, []string{".env", ".env.testing"}, []string{})
	streams(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, os.WriteFile(filepath.Join(checkout(t), ".env.testing"), []byte("SECRET=never-reaD\n"), 0o600))

	stdout, stderr := streams(t, "test", fixturePR, "--repo", fixtureSlug)
	reason := "sandbox.copy in " + profileFile + " names .env.testing, and the checkout " +
		checkout(t) + " holds a copy that differs from the sandbox's"
	assert.Equal(t, "experiment "+runner+"\n"+
		"  sandbox    "+sandboxPath+"\n"+
		"  recreated  per §5.1.6: "+reason+"\n"+
		"  env files  .env, .env.testing gitignored at the clone root\n"+
		"  in sandbox .env, .env.testing\n"+
		"env .env.testing\n"+recapLine, stderr)
	assert.Equal(t, []any{"sandbox " + sandboxPath + " recreated, per §5.1.6: " + reason}, honestyList(t, stdout))

	copied, err := os.ReadFile(filepath.Join(sandboxPath, ".env.testing"))
	require.NoError(t, err)
	assert.Equal(t, "SECRET=never-reaD\n", string(copied), "the recreation copied the checkout's file in")
}

// What must not rebuild a sandbox: an entry the checkout lacks stays absent, a
// directory is compared by presence only, and a copied file §5.1.3's setup
// rewrote is not compared with the checkout's.
func TestASandboxHoldingWhatItCanCopyIsNotRecreated(t *testing.T) {
	_, sandboxPath, runner, profileFile := envFixture(t, []string{".env"}, ".env", "vendor")
	dir := checkout(t)
	require.NoError(t, os.Remove(filepath.Join(dir, "vendor")))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor", "autoload.php"), []byte("one\n"), 0o600))
	rewrite := filepath.Join(t.TempDir(), "rewrite.sh")
	require.NoError(t, os.WriteFile(rewrite, []byte("#!/bin/sh\necho 'APP_KEY=generated' >> .env\n"), 0o700))
	rewriteProfile(t, profileFile, runner, []string{".env", ".env.testing", "vendor"}, []string{rewrite})
	streams(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor", "autoload.php"), []byte("two, longer\n"), 0o600))

	for range 2 {
		stdout, stderr := streams(t, "test", fixturePR, "--repo", fixtureSlug)
		assert.Equal(t, "experiment "+runner+"\n"+
			"  sandbox    "+sandboxPath+"\n"+
			"  env files  .env gitignored at the clone root\n"+
			"  in sandbox .env\n"+
			"env .env\n"+recapLine, stderr)
		assert.Equal(t, []any{}, honestyList(t, stdout))
	}

	// Presence is still asked of the directory: a sandbox that lost it is
	// stale.
	require.NoError(t, os.RemoveAll(filepath.Join(sandboxPath, "vendor")))
	stdout, _ := streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, []any{"sandbox " + sandboxPath + " recreated, per §5.1.6: sandbox.copy in " + profileFile +
		" names vendor, which the checkout " + dir + " holds and the sandbox does not"}, honestyList(t, stdout))
}

// When the sandbox cannot be rebuilt with a named file — here cr runs from a
// directory below the clone root, which §5.1.2 copies from, while the file
// sits at the root — the header and honesty name the file rather than staying
// silent.
func TestAHeaderNamesACopiedFileTheSandboxStillLacks(t *testing.T) {
	root, sandboxPath, runner, profileFile := envFixture(t, []string{".env.testing"}, ".env.testing")
	rewriteProfile(t, profileFile, runner, []string{".env.testing"}, []string{})
	below := filepath.Join(checkout(t), "below")
	require.NoError(t, os.MkdirAll(below, 0o700))
	restore := repoDir
	repoDir = func() (string, error) { return below, nil }
	t.Cleanup(func() { repoDir = restore })

	sentence := ".env.testing is gitignored at the clone root " + root + " and sandbox.copy in " + profileFile +
		" names it, but the sandbox does not hold it, so the suite runs without it"
	streams(t, "test", fixturePR, "--repo", fixtureSlug)
	stdout, stderr := streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, "experiment "+runner+"\n"+
		"  sandbox    "+sandboxPath+"\n"+
		"  env files  .env.testing gitignored at the clone root\n"+
		"  in sandbox none\n"+
		"  not copied "+sentence+"\n"+
		"env .env\n"+recapLine, stderr)
	assert.Equal(t, []any{sentence}, honestyList(t, stdout))
}
