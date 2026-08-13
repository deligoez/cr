package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// profilesDir writes one well-formed profile per entry of markers, keyed by
// profile id, and returns the directory holding them.
func profilesDir(t *testing.T, markers map[string][]string) string {
	t.Helper()
	dir := t.TempDir()
	for id, files := range markers {
		encoded, err := json.Marshal(map[string]any{
			"id":    id,
			"match": map[string]any{"files": files, "globs": []string{"**/*"}},
			"axes":  map[string]bool{"correctness": true},
		})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+".json"), encoded, 0o600))
	}
	return dir
}

// repoWith returns a repository directory carrying the named files, standing in
// for the worktree under review.
func repoWith(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range files {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600))
	}
	return root
}

// §2.4.1 makes selection automatic via `match.files`, so the repository decides:
// the profile whose marker file is there wins, and no profile is chosen for a
// repository carrying none. The second half is what keeps §2.4.3 true — a
// profile declaring no marker file matches nothing rather than everything, and
// a matcher reading an empty list as a wildcard would let it beat every real
// profile in every repository.
func TestSelectionFollowsTheMarkerFilesInTheRepository(t *testing.T) {
	dir := profilesDir(t, map[string][]string{
		"laravel-pest":  {"artisan"},
		"node":          {"package.json"},
		"empty-markers": {},
	})

	t.Run("the profile whose marker is present wins", func(t *testing.T) {
		selection, err := Select(dir, repoWith(t, "artisan"), "")
		require.NoError(t, err)

		assert.True(t, selection.Selected)
		assert.Equal(t, "laravel-pest", selection.Profile.ID)
		assert.Empty(t, selection.Tied)
	})

	t.Run("a profile declaring no marker is never selected automatically", func(t *testing.T) {
		selection, err := Select(dir, repoWith(t), "")
		require.NoError(t, err)

		assert.False(t, selection.Selected)
		assert.Empty(t, selection.Profile.ID)
		assert.Empty(t, selection.Tied)
	})
}

// §2.4.1 makes the automatic choice overridable by `profile` in the
// per-repository config, which §2.7 resolves through its layers before this
// package sees it. The override must beat a marker file that would otherwise
// decide, and it is the only way to reach a profile with an empty
// `match.files`, which §2.4.3 says applies only when named by configuration.
func TestConfigurationOverridesTheMarkerFiles(t *testing.T) {
	dir := profilesDir(t, map[string][]string{
		"laravel-pest":  {"artisan"},
		"empty-markers": {},
	})

	selection, err := Select(dir, repoWith(t, "artisan"), "empty-markers")
	require.NoError(t, err)

	assert.True(t, selection.Selected)
	assert.Equal(t, "empty-markers", selection.Profile.ID)
	assert.Empty(t, selection.Tied)
}

