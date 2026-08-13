package profile

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shippedProfilesDir writes every profile cr ships into a directory laid out
// like the one `cr init` fills, so selection is exercised against the files cr
// really writes rather than against profiles the test invented.
func shippedProfilesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for id, content := range Builtins() {
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+fileExt), []byte(content), 0o600))
	}
	return dir
}

// §2.4.5 fixes the shipped set at two profiles, and the exact set is the
// assertion rather than the presence of each. A third profile shipped by
// accident is a candidate in every repository §2.4.2 then has to count against
// the others, and a missing one leaves a whole class of repository unreviewable:
// without generic there is nothing to name when no language profile fits.
func TestV01ShipsExactlyTheTwoProfilesOf245(t *testing.T) {
	shipped := Builtins()

	assert.Equal(t, []string{genericID, laravelPestID}, slices.Sorted(maps.Keys(shipped)))
	for id, content := range shipped {
		// A profile cr ships and then refuses would abort with exit
		// code 3 every command that reads it.
		p, err := Parse(id+fileExt, []byte(content))
		require.NoError(t, err)
		assert.Equal(t, id, p.ID)
	}
}

// §2.4.3 keeps generic out of automatic selection, and the empty `match.files`
// on the shipped file is the entire mechanism. It is asserted against the file
// cr writes and through Select rather than against either alone, because a
// generic profile that could win automatically would beat every real profile in
// every repository the real one does not match — the fallback would silently
// become the default, and every review would run with the two lenses below
// missing.
func TestTheShippedGenericProfileIsNeverSelectedAutomatically(t *testing.T) {
	dir := shippedProfilesDir(t)

	shipped, err := Load(filepath.Join(dir, genericID+fileExt))
	require.NoError(t, err)
	assert.Empty(t, shipped.Match.Files)
	// It still owns source, or naming it would leave nothing to review.
	assert.NotEmpty(t, shipped.Match.Globs)

	t.Run("a repository no marker file matches selects nothing", func(t *testing.T) {
		selection, err := Select(dir, repoWith(t), "")
		require.NoError(t, err)

		assert.False(t, selection.Selected)
		assert.Empty(t, selection.Profile.ID)
		assert.Empty(t, selection.Tied)
	})

	t.Run("a repository a real profile matches selects that one", func(t *testing.T) {
		selection, err := Select(dir, repoWith(t, "artisan", "composer.json"), "")
		require.NoError(t, err)

		require.True(t, selection.Selected)
		assert.Equal(t, laravelPestID, selection.Profile.ID)
		assert.Empty(t, selection.Tied)
	})
}
