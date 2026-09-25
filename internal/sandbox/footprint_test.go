package sandbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// A sandbox's footprint counts the bytes of its regular files and follows no
// symbolic link, dates it from the directory's modification time when no
// baseline names a generation and from the generation when one does, and a
// pull request with no sandbox has no footprint.
func TestAFootprintCountsRegularFilesAndDatesTheSandbox(t *testing.T) {
	l := state.New(t.TempDir())
	require.NoError(t, l.Init())
	require.NoError(t, l.EnsurePR("acme", "shop", 7))
	absent, err := FootprintOf(l, "acme", "shop", 7, time.Now())
	require.NoError(t, err)
	assert.Nil(t, absent, "no sandbox, no footprint")

	path := l.Sandbox("acme", "shop", 7)
	require.NoError(t, os.MkdirAll(filepath.Join(path, "sub"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "a"), []byte("abc"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(path, "sub", "b"), []byte("hello"), 0o600))
	elsewhere := filepath.Join(t.TempDir(), "big")
	require.NoError(t, os.WriteFile(elsewhere, make([]byte, 1000), 0o600))
	require.NoError(t, os.Symlink(elsewhere, filepath.Join(path, "link")))
	modified := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(path, modified, modified))
	now := modified.Add(90 * time.Minute)

	measured, err := FootprintOf(l, "acme", "shop", 7, now)
	require.NoError(t, err)
	assert.Equal(t, &Footprint{Path: path, Bytes: 8, Since: modified, AgeSeconds: 5400}, measured,
		"3 + 5 bytes; the link to 1000 bytes counts nothing")

	built := time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)
	body, err := encodeBaseline(&Baseline{Head: "abc", Generation: built.Format(time.RFC3339Nano)})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(l.PRFile("acme", "shop", 7, state.FileSandboxBaseline), body, 0o600))
	measured, err = FootprintOf(l, "acme", "shop", 7, now)
	require.NoError(t, err)
	assert.Equal(t, built, measured.Since, "the generation is when Create built it")
	assert.Equal(t, int64(1800), measured.AgeSeconds)
}
