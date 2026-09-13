package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every unit named gets its fan-out directory under the pull request's state
// directory, and a second ensure keeps what a role already wrote there.
func TestEnsureFanOutCreatesOneDirectoryPerUnitAndKeepsWhatIsInIt(t *testing.T) {
	l := unlockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, held.EnsureFanOut(3, []string{"u1", "u2"}))
	require.NoError(t, held.Unlock())

	for _, unit := range []string{"u1", "u2"} {
		dir := l.FanOutDir("acme", "web", 42, 3, unit)
		assert.Equal(t, filepath.Join(l.PRDir("acme", "web", 42), "fanout", "3", unit), dir)
		info, err := os.Stat(dir)
		require.NoError(t, err, unit)
		assert.True(t, info.IsDir(), unit)
	}

	written := filepath.Join(l.FanOutDir("acme", "web", 42, 3, "u1"), "review-correctness.ndjson")
	require.NoError(t, os.WriteFile(written, []byte("{}\n"), 0o600))
	held, err = l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, held.EnsureFanOut(3, []string{"u1"}))
	require.NoError(t, held.Unlock())
	body, err := os.ReadFile(written)
	require.NoError(t, err)
	assert.Equal(t, "{}\n", string(body), "emitting a round twice discards nothing a role wrote")
}

// A unit id that is not one path segment is refused, and so is a round no
// brief has opened, and neither creates anything.
func TestEnsureFanOutRefusesAnIDThatCouldClimbAndARoundOfZero(t *testing.T) {
	l := unlockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	defer func() { require.NoError(t, held.Unlock()) }()

	for _, unit := range []string{"", ".", "..", "../u1", "u1/u2"} {
		assert.Errorf(t, held.EnsureFanOut(1, []string{unit}), "%q", unit)
	}
	assert.Error(t, held.EnsureFanOut(0, []string{"u1"}))

	_, err = os.Stat(filepath.Join(l.PRDir("acme", "web", 42), DirFanOut))
	assert.ErrorIs(t, err, os.ErrNotExist)
}
