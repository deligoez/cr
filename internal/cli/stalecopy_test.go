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

// rewritingFixture is envFixture's clone holding a gitignored `.env`, with a
// profile copying it and a §5.1.3 setup that appends to the sandbox's copy,
// the way `php artisan key:generate` does, and a sandbox already created.
func rewritingFixture(t *testing.T) (sandboxPath, runner, profileFile string) {
	t.Helper()
	_, sandboxPath, runner, profileFile = envFixture(t, []string{".env"}, ".env")
	rewrite := filepath.Join(t.TempDir(), "rewrite.sh")
	require.NoError(t, os.WriteFile(rewrite, []byte("#!/bin/sh\necho 'APP_KEY=generated' >> .env\n"), 0o700))
	rewriteProfile(t, profileFile, runner, []string{".env"}, []string{rewrite})
	streams(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
	return sandboxPath, runner, profileFile
}

// v0.2.2 QA D-S22b-2: a copied file the setup rewrote is compared by the
// checkout's presence, not skipped. Deleted from the sandbox, it makes the
// sandbox stale: `cr test` recreates it, the setup rewrites the file again, and
// the next run recreates nothing. A clone edit to that file is still not
// detected, which is the limitation the exemption keeps.
func TestATestRunRecreatesASandboxThatLostAFileSetupRewrote(t *testing.T) {
	sandboxPath, runner, profileFile := rewritingFixture(t)
	require.NoError(t, os.Remove(filepath.Join(sandboxPath, ".env")))

	reason := "sandbox.copy in " + profileFile + " names .env, which the checkout " +
		checkout(t) + " holds and the sandbox does not"
	unchanged := "experiment " + runner + "\n" +
		"  sandbox    " + sandboxPath + "\n" +
		"  env files  .env gitignored at the clone root\n" +
		"  in sandbox .env\n" +
		"env .env\n" + recapLine
	stdout, stderr := streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, "experiment "+runner+"\n"+
		"  sandbox    "+sandboxPath+"\n"+
		"  recreated  per §5.1.6: "+reason+"\n"+
		"  env files  .env gitignored at the clone root\n"+
		"  in sandbox .env\n"+
		"env .env\n"+recapLine, stderr)
	assert.Equal(t, []any{"sandbox " + sandboxPath + " recreated, per §5.1.6: " + reason}, honestyList(t, stdout))
	rewritten, err := os.ReadFile(filepath.Join(sandboxPath, ".env"))
	require.NoError(t, err)
	assert.Equal(t, "SECRET=never-read\nAPP_KEY=generated\n", string(rewritten), "the setup ran again on the new copy")

	stdout, stderr = streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, unchanged, stderr, "the recreated sandbox holds the file, so nothing is recreated")
	assert.Equal(t, []any{}, honestyList(t, stdout))

	require.NoError(t, os.WriteFile(filepath.Join(checkout(t), ".env"), []byte("SECRET=changed-in-clone\n"), 0o600))
	stdout, stderr = streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, unchanged, stderr, "a clone edit to a file the setup rewrote is not detected")
	assert.Equal(t, []any{}, honestyList(t, stdout))
	kept, err := os.ReadFile(filepath.Join(sandboxPath, ".env"))
	require.NoError(t, err)
	assert.Equal(t, "SECRET=never-read\nAPP_KEY=generated\n", string(kept))

	require.NoError(t, os.Remove(filepath.Join(checkout(t), ".env")))
	stdout, _ = streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, []any{}, honestyList(t, stdout), "nor is its removal from the clone")
	assert.FileExists(t, filepath.Join(sandboxPath, ".env"))
}

// D-S22b-2 through `cr probe run`: the sandbox that lost the rewritten file is
// recreated before §5.2.6's baselines, and a `cr test` after it recreates
// nothing.
func TestAProbeRunRecreatesASandboxThatLostAFileSetupRewrote(t *testing.T) {
	sandboxPath, runner, profileFile := rewritingFixture(t)
	require.NoError(t, os.Remove(filepath.Join(sandboxPath, ".env")))

	reason := "sandbox.copy in " + profileFile + " names .env, which the checkout " +
		checkout(t) + " holds and the sandbox does not"
	stdout, stderr := streams(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", writePatch(t, fixtureDiff), "--filter", "retries")
	assert.Equal(t, "experiment "+runner+" --only retries\n"+
		"  sandbox    "+sandboxPath+"\n"+
		"  recreated  per §5.1.6: "+reason+"\n"+
		"  env files  .env gitignored at the clone root\n"+
		"  in sandbox .env\n"+
		"  baseline   the whole suite runs first as the §5.2.2 baseline: "+runner+"\n"+
		strings.Repeat("env .env\n"+recapLine, 3), stderr)
	assert.Equal(t, []any{"sandbox " + sandboxPath + " recreated, per §5.1.6: " + reason}, honestyList(t, stdout))

	stdout, _ = streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, []any{}, honestyList(t, stdout))
}

// v0.2.2 QA D-S22b-1: `.env.testing` moved out of the clone after the sandbox
// copied it. The sandbox holding a named entry the checkout no longer holds is
// stale; the recreation leaves it absent from both, the runner reads `.env`,
// and the next run recreates nothing.
func TestATestRunRecreatesASandboxHoldingACopiedFileTheCheckoutLost(t *testing.T) {
	_, sandboxPath, runner, profileFile := envFixture(t, []string{".env", ".env.testing"}, ".env", ".env.testing")
	rewriteProfile(t, profileFile, runner, []string{".env", ".env.testing"}, []string{})
	streams(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
	require.FileExists(t, filepath.Join(sandboxPath, ".env.testing"))
	require.NoError(t, os.Remove(filepath.Join(checkout(t), ".env.testing")))

	reason := "sandbox.copy in " + profileFile + " names .env.testing, which the sandbox holds and the checkout " +
		checkout(t) + " does not"
	stdout, stderr := streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, "experiment "+runner+"\n"+
		"  sandbox    "+sandboxPath+"\n"+
		"  recreated  per §5.1.6: "+reason+"\n"+
		"  env files  .env gitignored at the clone root\n"+
		"  in sandbox .env\n"+
		"env .env\n"+recapLine, stderr)
	assert.Equal(t, []any{"sandbox " + sandboxPath + " recreated, per §5.1.6: " + reason}, honestyList(t, stdout))
	assert.NoFileExists(t, filepath.Join(sandboxPath, ".env.testing"))

	stdout, stderr = streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, "experiment "+runner+"\n"+
		"  sandbox    "+sandboxPath+"\n"+
		"  env files  .env gitignored at the clone root\n"+
		"  in sandbox .env\n"+
		"env .env\n"+recapLine, stderr, "an entry absent from both is not recreated again")
	assert.Equal(t, []any{}, honestyList(t, stdout))
}

// An entry neither the clone nor the sandbox holds is ignored, including one
// `sandbox.copy` came to name after the sandbox's baseline was recorded.
func TestAnEntryAbsentFromBothNeverRecreatesTheSandbox(t *testing.T) {
	_, sandboxPath, runner, profileFile := envFixture(t, []string{".env"}, ".env")
	rewriteProfile(t, profileFile, runner, []string{".env"}, []string{})
	streams(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
	rewriteProfile(t, profileFile, runner, []string{".env", ".env.testing"}, []string{})

	for range 2 {
		stdout, stderr := streams(t, "test", fixturePR, "--repo", fixtureSlug)
		assert.Equal(t, "experiment "+runner+"\n"+
			"  sandbox    "+sandboxPath+"\n"+
			"  env files  .env gitignored at the clone root\n"+
			"  in sandbox .env\n"+
			"env .env\n"+recapLine, stderr)
		assert.Equal(t, []any{}, honestyList(t, stdout))
	}
}

// A directory follows the same presence rule the other way: `vendor` removed
// from the clone after the sandbox copied it makes the sandbox stale once.
func TestATestRunRecreatesASandboxHoldingADirectoryTheCheckoutLost(t *testing.T) {
	_, sandboxPath, runner, profileFile := envFixture(t, []string{".env"}, ".env", "vendor")
	dir := checkout(t)
	require.NoError(t, os.Remove(filepath.Join(dir, "vendor")))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor", "autoload.php"), []byte("one\n"), 0o600))
	rewriteProfile(t, profileFile, runner, []string{".env", "vendor"}, []string{})
	streams(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
	require.DirExists(t, filepath.Join(sandboxPath, "vendor"))
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "vendor")))

	stdout, _ := streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, []any{"sandbox " + sandboxPath + " recreated, per §5.1.6: sandbox.copy in " + profileFile +
		" names vendor, which the sandbox holds and the checkout " + dir + " does not"}, honestyList(t, stdout))
	assert.NoDirExists(t, filepath.Join(sandboxPath, "vendor"))

	stdout, _ = streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, []any{}, honestyList(t, stdout))
}

// A named entry the clone lacks and §5.1.3's setup creates is recorded with
// the post-setup baseline, so the sandbox holding it is not recreated before
// every run.
func TestASandboxWhoseSetupCreatesACopiedFileIsNotRecreated(t *testing.T) {
	_, sandboxPath, runner, profileFile := envFixture(t, []string{".env"}, ".env")
	create := filepath.Join(t.TempDir(), "create.sh")
	require.NoError(t, os.WriteFile(create, []byte("#!/bin/sh\necho 'DB=testing' > .env.testing\n"), 0o700))
	rewriteProfile(t, profileFile, runner, []string{".env", ".env.testing"}, []string{create})
	streams(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)

	for range 2 {
		stdout, stderr := streams(t, "test", fixturePR, "--repo", fixtureSlug)
		assert.Equal(t, "experiment "+runner+"\n"+
			"  sandbox    "+sandboxPath+"\n"+
			"  env files  .env gitignored at the clone root\n"+
			"  in sandbox .env\n"+
			"env .env.testing\n"+recapLine, stderr)
		assert.Equal(t, []any{}, honestyList(t, stdout))
	}
}
