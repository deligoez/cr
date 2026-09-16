package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
)

// staleRoleBytes is the correctness role as the v0.2.2 release shipped it,
// read from the bytes internal/role embeds rather than typed here, together
// with the role this build ships. The pair is the fixture every case below
// starts from, and the require is what keeps the fixture a fixture: once the
// two are equal there is no stale file left to make, and every assertion under
// this file would pass while measuring nothing.
func staleRoleBytes(t *testing.T) (previous []byte, current string) {
	t.Helper()
	previous, err := os.ReadFile(filepath.Join("..", "role", "builtin", "shipped", "v0.2.2", "correctness.json"))
	require.NoError(t, err)
	current = role.Builtins()["correctness"]
	require.NotEqual(t, current, string(previous),
		"the fixture is only a test while v0.2.2's correctness role differs from this build's")
	return previous, current
}

// §2.5.2: an ejected role file byte-equal to a role an earlier release shipped
// carries no edit, so `cr init` replaces it with this release's and says which
// file it updated; the same file with one value edited is the user's, so it is
// left byte for byte and named. A role that was never ejected stays unejected,
// because §2.5.2 gives writing an absent role file to `--eject-roles` alone.
func TestInitUpdatesOnlyAnEjectedRoleNobodyEdited(t *testing.T) {
	previous, current := staleRoleBytes(t)

	initialised := func(t *testing.T, onDisk []byte) (printed map[string]any, file string) {
		t.Helper()
		root := crHome(t)
		file = filepath.Join(root, "roles", "correctness.json")
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
		require.NoError(t, os.WriteFile(file, onDisk, 0o600))
		require.NoError(t, json.Unmarshal([]byte(throughAPipe(t, "init")), &printed))
		return printed, file
	}

	t.Run("a byte-equal v0.2.2 file", func(t *testing.T) {
		printed, file := initialised(t, previous)

		assert.Equal(t, []any{file}, printed["updated"])
		assert.Equal(t, []any{}, printed["honesty"])
		after, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Equal(t, current, string(after))
	})

	t.Run("the same file with one key edited", func(t *testing.T) {
		edited := bytes.Replace(previous, []byte(`"title": "Correctness`), []byte(`"title": "Sharpness`), 1)
		require.NotEqual(t, previous, edited)
		printed, file := initialised(t, edited)

		assert.Equal(t, []any{}, printed["updated"])
		assert.Equal(t, []any{file + " matches no version of the correctness role cr has shipped, " +
			"so cr init left it as it is; the shipped role may have changed since, " +
			"and cr init --eject-roles with CR_HOME set to an empty directory writes it for comparison"},
			printed["honesty"])
		after, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Equal(t, edited, after)
	})

	t.Run("a file already at this release", func(t *testing.T) {
		printed, file := initialised(t, []byte(current))

		assert.Equal(t, []any{}, printed["updated"])
		assert.Equal(t, []any{}, printed["honesty"])
		after, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Equal(t, current, string(after))
	})

	t.Run("a role that was never ejected", func(t *testing.T) {
		root := crHome(t)
		var printed map[string]any
		require.NoError(t, json.Unmarshal([]byte(throughAPipe(t, "init")), &printed))

		assert.Equal(t, []any{}, printed["updated"])
		assert.Equal(t, []any{}, printed["honesty"])
		entries, err := os.ReadDir(filepath.Join(root, "roles"))
		require.NoError(t, err, "§2.2 lays out the directory either way")
		assert.Empty(t, entries, "§2.5.2 leaves writing an absent role file to --eject-roles")
	})
}

// §2.5.2's second half: a command that loads an ejected role file equal to an
// earlier release's built-in, and not to the current one, reports it — naming
// the file, the release and `cr init`.
//
// It is asked of `cr status` through the whole command, because a report the
// reader never sees is not a report: the sentence has to survive the honesty
// channel and the terminal rendering, and a package-level check of
// role.StaleDisclosures would pass with the wiring absent.
func TestACommandReportsAnEjectedRoleAnEarlierReleaseShipped(t *testing.T) {
	previous, current := staleRoleBytes(t)

	eject := func(t *testing.T, onDisk []byte) {
		t.Helper()
		roles := filepath.Join(os.Getenv(state.HomeEnv), "roles")
		require.NoError(t, os.MkdirAll(roles, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(roles, "correctness.json"), onDisk, 0o600))
	}
	sentence := func() string {
		return filepath.Join(os.Getenv(state.HomeEnv), "roles", "correctness.json") +
			" is the correctness role cr v0.2.2 shipped, unedited, and the shipped role has since " +
			"changed instructions; cr init updates the file to it, and until then every prompt cr " +
			"emits for this role carries the earlier release's framing"
	}

	t.Run("an earlier release's role is named", func(t *testing.T) {
		emptyRoundHome(t)
		eject(t, previous)
		said := sentence()

		report := readCompleteness(t)
		shown := throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--no-color")

		assert.Contains(t, report.Honesty, said, "§2.5.2: the document owes the report")
		assert.Contains(t, shown, said, "§2.5.2: and so does the terminal")
	})

	t.Run("the current role is not", func(t *testing.T) {
		emptyRoundHome(t)
		eject(t, []byte(current))

		report := readCompleteness(t)

		assert.NotContains(t, strings.Join(report.Honesty, "\n"), "cr v0.2.2 shipped",
			"a file equal to this build's role is up to date, not stale")
	})

	t.Run("no ejected role at all is not", func(t *testing.T) {
		emptyRoundHome(t)

		report := readCompleteness(t)

		assert.NotContains(t, strings.Join(report.Honesty, "\n"), "correctness role cr",
			"§2.5.4 resolves an unejected role from the binary, and §2.5.2's report is about a file")
	})
}
