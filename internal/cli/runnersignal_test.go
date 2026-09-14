package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// stoppedRunnerNotice is the §5.3.3 disclosure of a runner an earlier cr left
// alive, whole but for the process group number, which the test cannot know.
var stoppedRunnerNotice = regexp.MustCompile(
	`^a test runner an earlier cr run left running in process group [0-9]+ was killed before this run, per §5\.3\.3$`)

// assertStoppedRunnerNotice asserts one honesty entry is that disclosure.
func assertStoppedRunnerNotice(t *testing.T, notice any) {
	t.Helper()
	text, ok := notice.(string)
	require.True(t, ok, "a disclosure is a string")
	assert.Regexp(t, stoppedRunnerNotice, text)
}

// runnerGroupAlive reports whether any process of the process group group is
// still there to signal.
func runnerGroupAlive(t *testing.T, group int) bool {
	t.Helper()
	err := syscall.Kill(-group, 0)
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	require.NoError(t, err)
	return true
}

// awaitRunnerGroupGone polls until group has no process left, failing after a
// bounded wait rather than sleeping a fixed time: killed processes are reaped
// by init, which a loaded machine may do late.
func awaitRunnerGroupGone(t *testing.T, group int, what string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for runnerGroupAlive(t, group) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(-group, syscall.SIGKILL)
			require.FailNow(t, what, "process group %d was still alive after 15s", group)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// QA D-S06-5: a `cr probe run` that is stopped while its runner runs leaves no
// runner behind, and the next run starts on a clean sandbox.
//
// The runner writes its own pid, which is its process group, once it holds the
// mutated file and then sleeps, so the signal lands while the suite is running
// against the mutation. SIGINT and SIGTERM are caught: cr kills the group,
// reverts, records nothing and exits 4. SIGKILL cannot be caught: the group
// outlives cr, and the next `cr test` kills it before it recreates the sandbox
// or runs anything, and says so.
func TestAStoppedProbeLeavesNoRunnerAndTheNextRunStartsClean(t *testing.T) {
	for _, stop := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL} {
		t.Run(stop.String(), func(t *testing.T) {
			pidFile := filepath.Join(t.TempDir(), "runner.pid")
			prepared, fixture, sandboxPath, _ := probeFixture(t,
				onlyWhenMutated("  echo $$ > "+pidFile+".tmp && mv "+pidFile+".tmp "+pidFile+"\n  sleep 30\n"))
			patch := writePatch(t, fixtureDiff)
			mutated := filepath.Join(sandboxPath, "app.go")

			probing := spawnProbe(t, prepared, fixture, "--kind", "mutation", "--patch", patch)
			probing.awaitOnDisk(t, "the runner never started against the mutation", func() bool {
				_, err := os.Stat(pidFile)
				return err == nil
			})
			written, err := os.ReadFile(pidFile)
			require.NoError(t, err)
			group, err := strconv.Atoi(strings.TrimSpace(string(written)))
			require.NoError(t, err)
			require.Greater(t, group, 1)
			t.Cleanup(func() { _ = syscall.Kill(-group, syscall.SIGKILL) })

			require.NoError(t, probing.cmd.Process.Signal(stop))
			select {
			case <-probing.done:
			case <-time.After(30 * time.Second):
				probing.stop()
				require.FailNow(t, "cr probe run did not exit after "+stop.String(), "%s", probing.output.String())
			}
			assert.Empty(t, storedRecords(t, prepared, state.FileProbes), "a stopped probe records nothing")

			if stop == syscall.SIGKILL {
				require.True(t, runnerGroupAlive(t, group),
					"a SIGKILL ends cr alone, so the runner is what the next run has to find")
				reported := probeDocument(t, throughAPipe(t, "test", fixturePR, "--repo", fixtureSlug))
				assert.False(t, runnerGroupAlive(t, group),
					"the next run killed the runner before it returned, not merely before it exited")
				honesty, ok := reported["honesty"].([]any)
				require.True(t, ok)
				require.Len(t, honesty, 2)
				assertStoppedRunnerNotice(t, honesty[0])
				assert.Contains(t, honesty[1], "§5.1.6")
			} else {
				var exit *exec.ExitError
				require.ErrorAs(t, probing.err, &exit, "%s", probing.output.String())
				assert.Equal(t, ExitState, exit.ExitCode(), "%s", probing.output.String())
				assert.Contains(t, probing.output.String(), "cr received "+stop.String()+
					" while the test runner ran: the runner's process group was killed and nothing was recorded for the run")
				awaitRunnerGroupGone(t, group, "cr exited and left its runner running")
				restored, err := os.ReadFile(mutated)
				require.NoError(t, err)
				assert.Equal(t, fixtureSource, string(restored), "§5.3.3: the mutation was reverted before cr exited")

				reported := probeDocument(t, throughAPipe(t, "test", fixturePR, "--repo", fixtureSlug))
				assert.Equal(t, []any{}, reported["honesty"],
					"the next run found no runner to stop and a sandbox with nothing to recreate")
			}
			rebuilt, err := os.ReadFile(mutated)
			require.NoError(t, err)
			assert.Equal(t, fixtureSource, string(rebuilt), "the next run measured the head's own code")
		})
	}
}
