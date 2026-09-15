package cli

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/sandbox"
	"github.com/deligoez/cr/internal/state"
)

// killBeforeRunnerRelease names the variable that makes this package's test
// binary, run as cr, kill itself with SIGKILL between starting the runner of a
// mutation run and releasing it — after the start and before the runner's
// process group is recorded, the window §5.3.3's next run could not see into.
const killBeforeRunnerRelease = "CRTEST_KILL_BEFORE_RUNNER_RELEASE"

// init places that kill. It is an init rather than a line of TestMain so the
// hook reaches the process spawnHeldProbe starts, which runs TestMain's cr
// branch and nothing else of the package's tests. The mutation run is told
// apart from the baselines before it by what the sandbox holds.
func init() {
	if os.Getenv(runAsCR) == "" || os.Getenv(killBeforeRunnerRelease) == "" {
		return
	}
	sandbox.BeforeRunnerRelease = func(dir string) {
		held, err := os.ReadFile(filepath.Join(dir, "app.go"))
		if err == nil && string(held) == fixtureMutated {
			// The sleep holds the calling goroutine until the kill
			// lands. Measured without it on macOS: cr went on to record
			// the group before the signal ended it.
			_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
			time.Sleep(time.Minute)
		}
	}
}

// spawnHeldProbe starts `cr probe run` as spawnProbe does, with the kill above
// armed.
func spawnHeldProbe(t *testing.T, prepared state.Layout, fixture string, args ...string) *spawnedProbe {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)
	p := &spawnedProbe{done: make(chan struct{})}
	p.cmd = exec.Command(self,
		append([]string{"probe", "run", fixturePR, "--repo", fixtureSlug}, args...)...)
	p.cmd.Dir = fixture
	p.cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"TMPDIR=" + os.TempDir(),
		state.HomeEnv + "=" + prepared.Root(),
		runAsCR + "=1",
		killBeforeRunnerRelease + "=1",
	}
	p.cmd.Stdout, p.cmd.Stderr = &p.output, &p.output
	require.NoError(t, p.cmd.Start())
	p.started = time.Now()
	go func() {
		p.err = p.cmd.Wait()
		close(p.done)
	}()
	t.Cleanup(func() { p.stop() })
	return p
}

// awaitRunnerLockFree polls until no process holds the fixture's runner lock.
// Every process a run starts inherits that lock — the held process and the
// runner it would become alike — so a free lock is a run with no process left.
func awaitRunnerLockFree(t *testing.T, prepared state.Layout) {
	t.Helper()
	path := prepared.RunnerLockFile(fixtureOwner, fixtureProject, fixturePRNumber)
	deadline := time.Now().Add(15 * time.Second)
	for {
		probe, err := os.OpenFile(path, os.O_RDWR, 0)
		require.NoError(t, err)
		locked := syscall.Flock(int(probe.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		_ = probe.Close()
		if locked == nil {
			return
		}
		require.ErrorIs(t, locked, syscall.EWOULDBLOCK)
		if time.Now().After(deadline) {
			require.FailNow(t, "a process of the killed run still holds the runner lock after 15s")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// §5.3.3: a cr killed with SIGKILL after it started the test runner and before
// it recorded the runner's process group leaves no runner behind.
//
// The runner is held: cr starts its own binary in the runner's place and lets
// it become the runner only once the group is recorded, so a kill inside that
// window ends the held process too, having run nothing. The kill lands in the
// mutation run, with the mutation on disk, so a runner that did start would be
// measuring it — the pid file the runner script writes only against the
// mutation is what would say so. The next `cr test` then finds no runner to
// stop, recreates the sandbox the kill left mutated, and records its run as
// any run is recorded.
func TestACrKilledBeforeItsRunnerIsReleasedLeavesNoRunner(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "runner.pid")
	prepared, fixture, sandboxPath, observed := probeFixture(t,
		onlyWhenMutated("  echo $$ > "+pidFile+"\n  sleep 30\n"))
	patch := writePatch(t, fixtureDiff)
	mutated := filepath.Join(sandboxPath, "app.go")

	probing := spawnHeldProbe(t, prepared, fixture, "--kind", "mutation", "--patch", patch)
	select {
	case <-probing.done:
	case <-time.After(60 * time.Second):
		probing.stop()
		require.FailNow(t, "cr probe run was not killed", "%s", probing.output.String())
	}
	var exit *exec.ExitError
	require.ErrorAs(t, probing.err, &exit, "%s", probing.output.String())
	status, ok := exit.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	require.Equal(t, []any{true, syscall.SIGKILL}, []any{status.Signaled(), status.Signal()},
		"cr ended on the kill placed between the runner's start and its release: %s", probing.output.String())

	held, err := os.ReadFile(mutated)
	require.NoError(t, err)
	require.Equal(t, fixtureMutated, string(held), "cr was killed while the mutation was on disk")

	awaitRunnerLockFree(t, prepared)
	_, err = os.Stat(pidFile)
	assert.True(t, errors.Is(err, fs.ErrNotExist), "no runner started against the mutation: %v", err)
	seen, err := os.ReadFile(observed)
	require.NoError(t, err)
	runs := strings.Split(strings.TrimSuffix(string(seen), "---\n"), "---\n")
	require.NotEmpty(t, runs, "the baselines ran through the hold before the kill")
	for _, run := range runs {
		assert.Equal(t, fixtureSource, run, "every runner that started read the head's own source")
	}
	recorded, err := os.ReadFile(prepared.RunnerLockFile(fixtureOwner, fixtureProject, fixturePRNumber))
	require.NoError(t, err)
	assert.Empty(t, string(recorded), "the kill came before the group was recorded")
	assert.Empty(t, storedRecords(t, prepared, state.FileProbes), "a killed probe records nothing")

	before := storedRecords(t, prepared, state.FileRuns)
	shown := throughAPipe(t, "test", fixturePR, "--repo", fixtureSlug)
	reported := probeDocument(t, shown)
	assert.Equal(t, []any{
		"sandbox " + sandboxPath + " recreated, per §5.1.6: tracked files differ from the post-setup baseline: app.go",
	}, reported["honesty"], "the next run found no runner to stop, only the mutated sandbox to recreate")

	rebuilt, err := os.ReadFile(mutated)
	require.NoError(t, err)
	assert.Equal(t, fixtureSource, string(rebuilt), "the recreated sandbox holds no mutation")
	after := storedRecords(t, prepared, state.FileRuns)
	require.Len(t, after, len(before)+1, "the next run was recorded")
	assert.Equal(t, []any{float64(0), float64(4), float64(0)},
		[]any{after[len(after)-1]["exit_code"], after[len(after)-1]["tests_run"], after[len(after)-1]["tests_failed"]},
		"the ordinary path records the run and its counts")
}
