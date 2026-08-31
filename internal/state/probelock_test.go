package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// probeLocks returns an initialised layout to take §5.6's locks against.
func probeLocks(t *testing.T) Layout {
	t.Helper()
	l := New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, l.Init())
	return l
}

// §5.6.1: the lock is named after the absolute path of the repository under
// review **together with** the profile id, so two cr runs never share a test
// database and two unrelated repositories never block each other.
//
// Both halves are asserted through the lock rather than through its file name,
// because the name is not what the section promises — the exclusion is. A
// second repository and a second profile each proceed while the first lock is
// held, and a second run of the same pair does not: it waits, and the wait ends
// exactly when the holder releases.
func TestProbeRunsSerialisePerRepositoryAndProfile(t *testing.T) {
	l := probeLocks(t)
	const (
		here  = "/src/acme/web"
		there = "/src/acme/api"
	)

	first, err := l.LockProbe(here, "laravel-pest", time.Second)
	require.NoError(t, err)

	elsewhere, err := l.LockProbe(there, "laravel-pest", 200*time.Millisecond)
	require.NoError(t, err,
		"§5.6.1: two unrelated repositories never block each other")
	require.NoError(t, elsewhere.Unlock())

	otherProfile, err := l.LockProbe(here, "generic", 200*time.Millisecond)
	require.NoError(t, err,
		"§5.6.1: the profile id is part of the name, so another profile is another lock")
	require.NoError(t, otherProfile.Unlock())

	waiting := make(chan error, 1)
	go func() {
		second, err := l.LockProbe(here, "laravel-pest", 30*time.Second)
		if err == nil {
			err = second.Unlock()
		}
		waiting <- err
	}()

	select {
	case err := <-waiting:
		t.Fatalf("a second run took the lock while the first held it: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	require.NoError(t, first.Unlock())
	select {
	case err := <-waiting:
		assert.NoError(t, err, "the wait ends when the holder releases")
	case <-time.After(30 * time.Second):
		t.Error("the second run never took the lock the first had released")
	}
}

// §5.6.2: a held lock is waited on for up to `probe.lock_timeout_seconds` and
// then the run fails, rather than proceeding beside the holder.
//
// The wait is bounded here at a tenth of a second so the test costs nothing;
// the value is the caller's, and internal/cli resolves it from the setting.
// What is asserted is that the whole of it was spent — a refusal that gave up
// at once would pass a test that only looked at the error — and that the
// failure names the pair the lock is over, since a file name under ~/.cr is
// not what the reader has to do something about.
func TestAHeldProbeLockIsWaitedOnAndThenRefused(t *testing.T) {
	l := probeLocks(t)
	const (
		repository = "/src/acme/web"
		profile    = "laravel-pest"
		wait       = 100 * time.Millisecond
	)

	held, err := l.LockProbe(repository, profile, time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, held.Unlock()) })

	started := time.Now()
	second, err := l.LockProbe(repository, profile, wait)
	spent := time.Since(started)

	assert.Nil(t, second)
	var locked *ProbeLockedError
	require.ErrorAs(t, err, &locked, "§5.6.2: the wait ends in a refusal")
	assert.Equal(t, repository, locked.RepoPath)
	assert.Equal(t, profile, locked.ProfileID)
	assert.Equal(t, wait, locked.Waited)
	assert.Contains(t, locked.Error(), "probe.lock_timeout_seconds",
		"the refusal names the setting the reader would raise")
	assert.GreaterOrEqual(t, spent, wait,
		"§5.6.2 waits for the timeout; a refusal that gave up at once never waited")
}

// An advisory lock binds to an inode, so the released file is kept for the
// reason Lock.Unlock keeps its own: unlinking it would let the next waiter open
// the same path, receive a fresh inode, and run beside the holder while every
// call reported success.
func TestReleasingTheProbeLockKeepsItsFile(t *testing.T) {
	l := probeLocks(t)

	held, err := l.LockProbe("/src/acme/web", "laravel-pest", time.Second)
	require.NoError(t, err)
	require.NoError(t, held.Unlock())

	info, err := os.Stat(l.ProbeLockFile("/src/acme/web", "laravel-pest"))
	require.NoError(t, err, "the released lock file must survive: it is the inode the next waiter locks")
	assert.False(t, info.IsDir())
	assert.Equal(t,
		filepath.Join(l.LocksDir(), DirProbeLocks, "src", "acme", "web", "laravel-pest.lock"),
		l.ProbeLockFile("/src/acme/web", "laravel-pest"),
		"§2.2: the lock lives under the state root, never beside the repository it names")
}

// §5.6.3: the lock covers cr's own runs and the warning says so, naming the
// repository whose other runs it cannot see.
//
// The sentence is derived from the lock's own halves rather than composed at
// the call site, so what a reader is told and what the lock actually guards
// cannot come apart — a warning naming some other checkout would be worse than
// none, because it would read as a clearance for this one.
func TestTheProbeLockSaysWhatItDoesNotCover(t *testing.T) {
	l := probeLocks(t)

	held, err := l.LockProbe("/src/acme/web", "laravel-pest", time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, held.Unlock()) })

	warning := held.CollisionWarning()
	assert.Contains(t, warning, "§5.6.3")
	assert.Contains(t, warning, "/src/acme/web",
		"the warning names the repository whose other runs the lock does not cover")
	assert.Contains(t, warning, "cr's own runs only")
}
