package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §5.6.1 names the probe lock after the absolute path of the repository under
// review and the profile id, and what the lock guards — the test database the
// profile configures — belongs to the clone rather than to the state root. So a
// `cr test` or a `cr probe run` waits for a run on the same clone whose
// `CR_HOME` is another, and §5.6.2 refuses it with exit 4 once
// `probe.lock_timeout_seconds` has passed, exactly as within one root; a run on
// a different clone under that other root does not hold it up at all.
//
// v0.2.0 kept the lock under `CR_HOME` alone, and two runs with different state
// roots ran the suite against one clone at once.
//
// The other root's run is the lock it takes, held through a layout of its own
// root the way `cr` resolves one from `CR_HOME`. What the command ran is read
// off the runner's log: every run that proceeded adds a recap to it, and the
// refused one adds none.
func TestARunUnderAnotherStateRootWaitsForTheClonesProbeLock(t *testing.T) {
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
			t.Setenv("CR_PROBE_LOCK_TIMEOUT_SECONDS", "1")
			args := command.args(t)
			recaps := func() int {
				body, err := os.ReadFile(log)
				if os.IsNotExist(err) {
					return 0
				}
				require.NoError(t, err)
				return strings.Count(string(body), "---\n")
			}

			elsewhere := state.New(filepath.Join(t.TempDir(), "another-cr-home"))
			require.NoError(t, elsewhere.Init())
			require.NotEqual(t, prepared.Root(), elsewhere.Root())

			anotherClone, err := elsewhere.LockProbe(filepath.Join(t.TempDir(), "another-clone"), "qa", time.Second)
			require.NoError(t, err)
			require.NoError(t, runCLI(t, args...), "§5.6.1: a run on another clone is not held up")
			require.NoError(t, anotherClone.Unlock())
			perRun := recaps()
			require.Positive(t, perRun, "and its suite ran")

			held, err := elsewhere.LockProbe(root, "qa", time.Second)
			require.NoError(t, err)
			started := time.Now()
			err = runCLI(t, args...)
			waited := time.Since(started)

			var locked *state.ProbeLockedError
			require.ErrorAs(t, err, &locked, "§5.6.2: the run waited for the clone's lock and gave up")
			assert.Equal(t, state.ProbeLockedError{RepoPath: root, ProfileID: "qa", Waited: time.Second}, *locked)
			assert.GreaterOrEqual(t, waited, time.Second, "the refusal came after the wait, not instead of it")
			assert.Equal(t, ExitState, exitCodeFor(err), "§5.6.2 fails with exit code 4")
			assert.Equal(t, perRun, recaps(), "no suite ran while the other root's run held the clone")

			require.NoError(t, held.Unlock())
			require.NoError(t, runCLI(t, args...), "with the lock released the same run proceeds")
			assert.Greater(t, recaps(), perRun, "and its suite ran")
		})
	}
}
