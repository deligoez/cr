package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §5.6.1 names the probe lock after the absolute path of the repository under
// review, so a `cr test` or a `cr probe run` started from a subdirectory of a
// checkout waits for one started at its root, and §5.6.2 refuses it with exit 4
// once `probe.lock_timeout_seconds` has passed.
//
// Audit round 5 found the lock named after the directory cr was started from:
// with the root's lock held, a run from `<root>/internal` took a lock of its own
// at once, and two runs shared the one test database the profile configures.
//
// The lock is held here under the root's own path with symbolic links resolved,
// which is the name a run started at the root takes, and the refused run is
// read for the pair it waited on as whole values. The runner's log is asserted
// absent, so the refusal is shown to come before any suite ran; and the lock is
// then released and the same command run again from the same subdirectory, so
// what refused it is shown to be the lock and nothing else.
func TestARunStartedInASubdirectoryWaitsForTheRootsProbeLock(t *testing.T) {
	commands := []struct {
		name string
		args func(t *testing.T) []string
	}{
		{"test", func(*testing.T) []string {
			return []string{"test", fixturePR, "--repo", fixtureSlug}
		}},
		{"probe_run", func(t *testing.T) []string {
			return []string{"probe", "run", fixturePR, "--repo", fixtureSlug,
				"--kind", "mutation", "--patch", writePatch(t, fixtureDiff)}
		}},
	}
	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			prepared, fixture, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
			root, err := filepath.EvalSymlinks(fixture)
			require.NoError(t, err)
			// An ignored directory the fixture already holds, so the
			// checkout's status is what the fixture made it.
			below := filepath.Join(fixture, "built")
			require.DirExists(t, below)
			restore := repoDir
			repoDir = func() (string, error) { return below, nil }
			t.Cleanup(func() { repoDir = restore })
			t.Setenv("CR_PROBE_LOCK_TIMEOUT_SECONDS", "1")
			args := command.args(t)

			held, err := prepared.LockProbe(root, "qa", time.Second)
			require.NoError(t, err)
			started := time.Now()
			err = runCLI(t, args...)
			waited := time.Since(started)

			var locked *state.ProbeLockedError
			require.ErrorAs(t, err, &locked, "§5.6.2: the run waited for the root's lock and gave up")
			assert.Equal(t, root, locked.RepoPath, "§5.6.1: the lock is named after the repository root")
			assert.Equal(t, "qa", locked.ProfileID)
			assert.Equal(t, time.Second, locked.Waited)
			assert.GreaterOrEqual(t, waited, time.Second, "the refusal came after the wait, not instead of it")
			assert.Equal(t, ExitState, exitCodeFor(err), "§5.6.2 fails with exit code 4")
			assert.NoFileExists(t, log, "no suite ran while the lock was held")

			require.NoError(t, held.Unlock())
			require.NoError(t, runCLI(t, args...), "with the lock released the same run proceeds")
			_, err = os.Stat(log)
			require.NoError(t, err, "and its suite ran")
		})
	}
}
