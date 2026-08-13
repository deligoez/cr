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

// §2.4.2 settles an overlap by count: the profile matching the greatest number
// of marker files wins. Both orders are exercised, because a matcher that
// simply keeps the first or the last match agrees with the rule in one of them
// and contradicts it in the other.
func TestTheGreatestMarkerCountWins(t *testing.T) {
	for _, tc := range []struct {
		name   string
		winner string
		loser  string
	}{
		{"the greater count is read last", "z-greater", "a-lesser"},
		{"the greater count is read first", "a-greater", "z-lesser"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := profilesDir(t, map[string][]string{
				tc.winner: {"artisan", "composer.json"},
				tc.loser:  {"artisan"},
			})

			selection, err := Select(dir, repoWith(t, "artisan", "composer.json"), "")
			require.NoError(t, err)

			assert.True(t, selection.Selected)
			assert.Equal(t, tc.winner, selection.Profile.ID)
			assert.Empty(t, selection.Tied)
		})
	}
}

// The profiles directory is the whole candidate set, so cr must tell "there is
// nothing to select from" apart from "the candidates could not be read". A
// state root without one yet holds no profile, which §2.4.4 already answers,
// while a path cr cannot list hides an unknown number of them: reading that as
// no match would disable axes on evidence cr never obtained, so it aborts as a
// malformed file instead, naming the path.
func TestAProfilesDirectoryIsEitherReadOrReported(t *testing.T) {
	t.Run("a missing directory holds no profile", func(t *testing.T) {
		selection, err := Select(filepath.Join(t.TempDir(), "profiles"), repoWith(t, "artisan"), "")
		require.NoError(t, err)

		assert.False(t, selection.Selected)
		assert.Empty(t, selection.Tied)
	})

	t.Run("a directory cr cannot list is named", func(t *testing.T) {
		notADirectory := filepath.Join(t.TempDir(), "profiles")
		require.NoError(t, os.WriteFile(notADirectory, []byte("{}"), 0o600))

		_, err := Select(notADirectory, repoWith(t, "artisan"), "")

		var malformed *MalformedError
		require.ErrorAs(t, err, &malformed)
		assert.Equal(t, notADirectory, malformed.File)
	})
}

