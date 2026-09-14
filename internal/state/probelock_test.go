package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// probeLocks returns an initialised layout to take §5.6's locks against.
//
// The temporary directory the second of §5.6.1's two flocks lives in is moved
// to one of the test's own. Those locks are shared by every process resolving
// the same os.TempDir, so a fixed pair such as /src/acme/web held here would
// otherwise make a second `go test` of this package on the machine wait on it.
func probeLocks(t *testing.T) Layout {
	t.Helper()
	l := New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, l.Init())
	t.Setenv("TMPDIR", t.TempDir())
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
	assert.Equal(t,
		"another cr run holds the probe lock for /src/acme/web under profile laravel-pest: "+
			"waited 100ms, which is probe.lock_timeout_seconds",
		locked.Error(),
		"the refusal names the pair and the setting, and leaves the step to ProbeLockedHint")
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

// §5.6.1 names the lock after the checkout and the profile, and what it guards
// — the test database the profile configures — belongs to the checkout rather
// than to the state root. So two runs on one clone under two `CR_HOME` roots
// serialise as two runs under one root do: the second waits the whole timeout
// and is refused with the pair it waited on, and the lock it did take under its
// own root goes with the refusal. A second clone under the other root is not
// held up, and once the holder releases, the other root's run proceeds.
func TestProbeRunsSerialisePerCloneAcrossStateRoots(t *testing.T) {
	l := probeLocks(t)
	other := New(filepath.Join(t.TempDir(), "other-cr"))
	require.NoError(t, other.Init())
	const (
		here    = "/src/acme/web"
		profile = "laravel-pest"
		wait    = 100 * time.Millisecond
	)

	held, err := l.LockProbe(here, profile, time.Second)
	require.NoError(t, err)

	started := time.Now()
	second, err := other.LockProbe(here, profile, wait)
	spent := time.Since(started)

	assert.Nil(t, second)
	var locked *ProbeLockedError
	require.ErrorAs(t, err, &locked, "§5.6.2: a run under another state root waits for the clone's lock")
	assert.Equal(t, ProbeLockedError{RepoPath: here, ProfileID: profile, Waited: wait}, *locked)
	assert.GreaterOrEqual(t, spent, wait, "the refusal came after the wait, not instead of it")

	under := flock.New(other.ProbeLockFile(here, profile))
	taken, err := under.TryLock()
	require.NoError(t, err)
	assert.True(t, taken, "the refused run released the lock it had taken under its own state root")
	require.NoError(t, under.Unlock())

	elsewhere, err := other.LockProbe("/src/acme/api", profile, wait)
	require.NoError(t, err, "§5.6.1: two different clones never block each other")
	require.NoError(t, elsewhere.Unlock())

	require.NoError(t, held.Unlock())
	after, err := other.LockProbe(here, profile, wait)
	require.NoError(t, err, "with the holder released, the run under the other root proceeds")
	require.NoError(t, after.Unlock())
}

// The lock under the temporary directory is taken after the one under the state
// root, never before: a run waiting on its own root's lock holds nothing a run
// under another root could be kept waiting by. Every run taking the two in one
// order is what keeps two runs from each holding one while waiting on the other.
//
// The state root's lock is held here directly, and the clone's lock is tried
// while LockProbe is still waiting on it.
func TestTheClonesProbeLockIsTakenAfterTheStateRootsLock(t *testing.T) {
	l := probeLocks(t)
	const (
		here    = "/src/acme/web"
		profile = "laravel-pest"
	)
	require.NoError(t, os.MkdirAll(filepath.Dir(l.ProbeLockFile(here, profile)), 0o700))
	home := flock.New(l.ProbeLockFile(here, profile))
	taken, err := home.TryLock()
	require.NoError(t, err)
	require.True(t, taken)
	t.Cleanup(func() { assert.NoError(t, home.Unlock()) })

	waiting := make(chan error, 1)
	go func() {
		_, err := l.LockProbe(here, profile, time.Second)
		waiting <- err
	}()
	time.Sleep(200 * time.Millisecond)

	clonePath, err := cloneLockFile(here, profile)
	require.NoError(t, err)
	clone := flock.New(clonePath)
	taken, err = clone.TryLock()
	require.NoError(t, err)
	assert.True(t, taken, "a run waiting on its state root's lock holds no lock on the clone")
	require.NoError(t, clone.Unlock())

	var locked *ProbeLockedError
	assert.ErrorAs(t, <-waiting, &locked)
}

// Releasing the lock releases both flocks, so a run under the same state root
// and a run under another one each take it afterwards at once.
func TestReleasingTheProbeLockReleasesItForEveryStateRoot(t *testing.T) {
	l := probeLocks(t)
	other := New(filepath.Join(t.TempDir(), "other-cr"))
	require.NoError(t, other.Init())

	held, err := l.LockProbe("/src/acme/web", "laravel-pest", time.Second)
	require.NoError(t, err)
	require.NoError(t, held.Unlock())

	for name, layout := range map[string]Layout{"the same state root": l, "another state root": other} {
		again, err := layout.LockProbe("/src/acme/web", "laravel-pest", 100*time.Millisecond)
		require.NoError(t, err, name)
		require.NoError(t, again.Unlock(), name)
	}
}

// The clone's lock file is named by a digest of the pair, so neither half can
// lead it out of the temporary directory, the same checkout spelled with a `..`
// is the same lock, and two pairs whose halves join to one string are not.
func TestTheClonesProbeLockFileStaysInTheTemporaryDirectory(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	climbing, err := cloneLockFile("/src/../../../etc", "../../../x")
	require.NoError(t, err)
	assert.Equal(t, os.TempDir(), filepath.Dir(climbing), "no half leads the lock file out of os.TempDir")
	assert.Regexp(t, `^cr-probe-[0-9a-f]{16}\.lock$`, filepath.Base(climbing))

	cleaned, err := cloneLockFile("/src/acme/web", "laravel-pest")
	require.NoError(t, err)
	spelled, err := cloneLockFile("/src/acme/../acme/web", "laravel-pest")
	require.NoError(t, err)
	assert.Equal(t, cleaned, spelled, "one checkout is one lock however its path is spelled")

	splitLate, err := cloneLockFile("/src/acme", "web/laravel-pest")
	require.NoError(t, err)
	assert.NotEqual(t, cleaned, splitLate, "the boundary between the halves is part of the name")

	anotherProfile, err := cloneLockFile("/src/acme/web", "generic")
	require.NoError(t, err)
	assert.NotEqual(t, cleaned, anotherProfile, "§5.6.1: the profile id is part of the name")
}
