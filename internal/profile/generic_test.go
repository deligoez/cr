package profile

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/deligoez/cr/internal/axis"
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

// §2.4.5's table is the shipped set, and the exact set is the assertion rather
// than the presence of each. A profile shipped by accident is a candidate in
// every repository §2.4.2 then has to count against the others, and a missing
// one leaves a whole class of repository unreviewable: without generic there is
// nothing to name when no language profile fits.
func TestCrShipsExactlyTheProfilesOf245sTable(t *testing.T) {
	shipped := Builtins()

	assert.Equal(t, []string{genericID, goID, jestID, laravelPestID, rustID, typescriptID}, slices.Sorted(maps.Keys(shipped)))
	for id, content := range shipped {
		// A profile cr ships and then refuses would abort with exit
		// code 3 every command that reads it.
		p, err := Parse(id+fileExt, []byte(content))
		require.NoError(t, err)
		assert.Equal(t, id, p.ID)
	}
}

// §2.4.5 pins a property rather than a count: every shipped profile other than
// generic carries what the probe machinery needs to work at all. A profile
// missing one of these does not fail loudly — it answers every probe
// `inconclusive` or refuses every filter — so the property is checked on every
// shipped profile, the next one included, rather than trusted to review.
func TestEveryShippedLanguageProfileCanRunAProbe(t *testing.T) {
	for id, content := range Builtins() {
		if id == genericID {
			continue
		}
		t.Run(id, func(t *testing.T) {
			p, err := Parse(id+fileExt, []byte(content))
			require.NoError(t, err)
			assert.NotEmpty(t, p.Tests.Cmd, "tests.cmd")
			assert.NotEmpty(t, p.Tests.CountPattern, "tests.count_pattern")
			assert.NotEmpty(t, p.Tests.FailedPattern, "tests.failed_pattern")
			assert.NotEmpty(t, p.Tests.FilterFlag, "tests.filter_flag")
			assert.NotEmpty(t, p.Tests.ProbePathTemplate, "tests.probe_path_template")
		})
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

// lensesTheProfileTakesOut names every lens §4.5.4 has to report as not run
// because of the profile alone. §4.5's activation is a later obligation, so
// this is the test's own reading of the conditions that will drive it, written
// out here rather than in the package: what a profile settles, not what a run
// settles. The intent axis of §4.5.3 and a role skipped per §4.6.4 are outside
// it, because no profile field decides either.
func lensesTheProfileTakesOut(p *Profile) []string {
	out := make([]string, 0, 2)
	for _, id := range axis.IDs() {
		if !p.Axes[id] {
			out = append(out, "axis "+id+" disabled by axes")
		}
	}
	if len(p.Tests.Cmd) == 0 {
		out = append(out, "axis test disabled by an absent tests.cmd, per §4.5.2")
	}
	if p.Symbols.Lang == "" {
		out = append(out, "reinvention half of §4.3.1 unavailable by an absent symbols.lang")
	}
	return out
}

// §2.4.3 leaves configuration as the only way to reach generic, and what naming
// it buys is a review with exactly two lenses honestly out: the test axis,
// because §4.5.2 disables it on a profile declaring no `tests.cmd`, and the
// reinvention half of §4.3.1, because there is no `symbols.lang` to build a
// symbol index from. Exactness is the assertion in both directions. A third
// entry would mean generic gave up a lens it could have run — the `axes` block
// answers all four ids of §1.5 with true, so nothing here is switched off by
// declaration, which is what keeps the disabling attributable to the absent
// field §4.5.2 names and lets a user who adds a runner to their own copy get
// the axis back. Fewer than two would mean cr claimed a lens it cannot support.
//
// The other shipped profile is read the same way as a control, so the two
// entries are generic's own emptiness rather than a verdict this reading
// returns for any profile.
func TestSelectingGenericByConfigurationTakesOutExactlyTwoLenses(t *testing.T) {
	dir := shippedProfilesDir(t)

	// The repository carries a marker of the other profile, so the choice
	// is the configuration's and nothing else's.
	selection, err := Select(dir, repoWith(t, "artisan"), genericID)
	require.NoError(t, err)
	require.True(t, selection.Selected)
	require.Equal(t, genericID, selection.Profile.ID)

	assert.Equal(t, []string{
		"axis test disabled by an absent tests.cmd, per §4.5.2",
		"reinvention half of §4.3.1 unavailable by an absent symbols.lang",
	}, lensesTheProfileTakesOut(&selection.Profile))

	laravel, err := Load(filepath.Join(dir, laravelPestID+fileExt))
	require.NoError(t, err)
	assert.Empty(t, lensesTheProfileTakesOut(&laravel))
}

// §2.4.3 and §2.4.2 are one mechanism seen from the tie side. generic's empty
// `match.files` has to count as zero matches rather than as a match on
// everything, and the tie path is where the difference first becomes an abort:
// read as universal, generic would draw with laravel-pest in a Laravel
// repository and with itself-at-zero in every other, so cr would exit 3 naming
// the shipped pair exactly where §2.4.4 requires it to report and carry on. The
// two repositories below are the two shapes that abort under that reading.
func TestTheShippedGenericProfileNeverJoinsATie(t *testing.T) {
	dir := shippedProfilesDir(t)

	for _, tc := range []struct {
		name  string
		files []string
	}{
		{"a repository the other shipped profile matches", []string{"artisan", "composer.json"}},
		{"a repository neither shipped profile matches", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selection, err := Select(dir, repoWith(t, tc.files...), "")
			require.NoError(t, err)

			assert.Empty(t, selection.Tied)
			assert.NoError(t, selection.Err())
		})
	}
}
