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
