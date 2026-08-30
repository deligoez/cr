package role

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shippedAxes is §2.5.1's list of the four defaults, written out here rather
// than read from the package, because a guard that read the value it judges
// would pass whatever the package happened to say. The last row is the one
// worth reading twice: §2.5.1 names the role test-adequacy while §1.5 names the
// axis test, so the two strings differ on purpose and neither may be spelled as
// the other.
var shippedAxes = map[string]string{
	"intent-coverage": axis.Intent,
	"correctness":     axis.Correctness,
	"convention":      axis.Convention,
	"test-adequacy":   axis.Test,
}

// §2.5.1 requires v0.1 to ship one role per axis, and §4.5.1 makes a role
// active through its axis: an axis no shipped role serves is a lens that never
// looks, which §4.5.4 must then report as a hole in coverage rather than as a
// missing file. Two roles on one axis is the opposite fault and just as quiet,
// because §2.5.5 lets a corpus hold several and §6.4.2 would simply order them
// — the duplicate reads as a richer review while the axis it was taken from
// goes unlooked at.
//
// Each file is parsed under the name cr writes it as, by the same loader that
// reads a user's own role, so a default that would abort per §2.5.3 fails here
// rather than in front of a user.
func TestTheShippedRolesCoverEveryAxisExactlyOnce(t *testing.T) {
	shipped := Builtins()
	require.Len(t, shipped, len(axis.IDs()))
	require.Len(t, shippedAxes, len(axis.IDs()))

	filled := make(map[string]string, len(shipped))
	for id, content := range shipped {
		r, err := Parse(id+".json", []byte(content))
		require.NoError(t, err, "cr ships no role its own loader would refuse")

		require.Contains(t, shippedAxes, id, "§2.5.1 fixes which four ids ship")
		assert.Equal(t, id, r.ID)
		assert.True(t, axis.Valid(r.Axis), "%s names %q, which is not an axis id", id, r.Axis)
		assert.Equal(t, shippedAxes[id], r.Axis)
		assert.NotEmpty(t, r.Title, "§2.5 shows the title in prompts and progress output")
		assert.NotEmpty(t, r.Focus, "§4.6.1 appends focus to the prompt")
		// Empty means all per §2.5, and a default has no basis to exclude
		// a profile it has never seen.
		assert.Empty(t, r.Profiles)

		if taken, already := filled[r.Axis]; already {
			t.Errorf("%s and %s both serve the %s axis", taken, id, r.Axis)
		}
		filled[r.Axis] = id
	}

	for _, id := range axis.IDs() {
		assert.Contains(t, filled, id, "no shipped role serves the %s axis", id)
	}
}

// §2.5.2's byte-identity is a claim about the files in this repository, so the
// guard has to read them. An ejected tree compared against Builtins() alone
// would still pass a Builtins() that marshalled a Role struct instead of
// embedding the file: both sides would move together, and what `cr init
// --eject-roles` wrote would quietly become whatever the encoder chose that
// day rather than the reviewed file. Reading builtin/<id>.json off disk is the
// only comparison that can see that, and `go test` runs a package's tests with
// the package directory as the working directory, so the relative path names
// this package's own corpus and no copy of it.
//
// The directory listing is the other direction. A file added to builtin/ and
// never embedded ships in no binary and is ejected by nothing, and Builtins()
// on its own has no way to say so.
func TestBuiltinsReturnsTheFilesOnDisk(t *testing.T) {
	const dir = "builtin"

	shipped := Builtins()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, len(shipped), "%s holds the shipped roles and nothing else", dir)

	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".json")
		content, ok := shipped[id]
		require.True(t, ok, "%s is embedded by nothing", entry.Name())

		onDisk, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, err)
		assert.Equal(t, string(onDisk), content, "%s is not the bytes of its own file", entry.Name())
	}
}
