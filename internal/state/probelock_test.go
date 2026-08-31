package state

import (
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
