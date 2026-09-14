package profile

import (
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.4.4 is two obligations that pull against each other: disable every axis
// that requires a profile, and do not guess. Exactness in both directions is
// therefore the assertion. A third entry would be cr reporting a lens as dead
// that a missing profile does not touch — the intent axis of §4.5.3 turns on an
// issue key and the correctness axis of §4.2 on claims and units, so a
// repository with no profile still gets both — and that is the guess §2.4.4
// forbids, aimed at the review rather than at the language. Fewer than two
// would be worse: the round would come back complete with the test axis and the
// reinvention search counted as having looked when neither could.
//
// The repository carries no marker file, which is the state §2.4.4 speaks of,
// and the shipped profiles are the candidate set, so nothing matching is the
// real outcome of the real files rather than an outcome the test arranged.
func TestARepositoryNoProfileMatchesDisablesOnlyTheAxesThatNeedOne(t *testing.T) {
	dir := shippedProfilesDir(t)

	selection, err := Select(dir, repoWith(t), "")
	require.NoError(t, err)
	require.False(t, selection.Selected)
	// §2.4.4 reports and carries on, so nothing here is an abort.
	require.NoError(t, selection.Err())

	missing, none := selection.Missing()
	require.True(t, none)
	assert.Equal(t, []string{axis.Test}, missing.Disabled)
	assert.Equal(t, []string{"convention/reinvention"}, missing.Unavailable)

	t.Run("a selected profile is not the missing state", func(t *testing.T) {
		selected, err := Select(dir, repoWith(t, "artisan", "composer.json"), "")
		require.NoError(t, err)
		require.True(t, selected.Selected)

		_, none := selected.Missing()
		assert.False(t, none)
	})

	t.Run("a tie is not the missing state", func(t *testing.T) {
		tied, err := Select(
			profilesDir(t, map[string][]string{
				"laravel-pest": {"artisan"},
				"symfony":      {"artisan"},
			}),
			repoWith(t, "artisan"),
			"",
		)
		require.NoError(t, err)
		require.NotEmpty(t, tied.Tied)

		// §2.4.2 aborts here, so a report saying the run carried on
		// with two lenses out would describe a run that never happened.
		_, none := tied.Missing()
		assert.False(t, none)
	})
}
