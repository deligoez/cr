package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.6's `profiles` row read by ForProfile: an empty list applies under every
// profile, a named list only under the profiles it names, and neither under a
// round no profile matched unless the list is empty.
//
// The per-repository layer's `shared` is scoped to alpha and shadows a global
// `shared` scoped to nothing. Resolution runs first, per §2.6 item 2, so under
// beta neither runs: the global rule was overridden whole, and the rule that
// won does not apply there.
func TestARuleAppliesOnlyUnderTheProfilesItNames(t *testing.T) {
	corpus, err := Resolve(
		rulesDir(t,
			ruleDoc("scoped", map[string]any{"profiles": []string{"alpha", "gamma"}}),
			ruleDoc("shared", map[string]any{"profiles": []string{"alpha"}})),
		rulesDir(t, ruleDoc("everywhere", nil), ruleDoc("shared", nil)),
		profileFile, nil,
	)
	require.NoError(t, err)
	require.Equal(t, []string{"scoped", "shared", "everywhere"}, ids(corpus))

	cases := map[string][]string{
		"alpha": {"scoped", "shared", "everywhere"},
		"gamma": {"scoped", "everywhere"},
		"beta":  {"everywhere"},
		"":      {"everywhere"},
	}
	for profileID, want := range cases {
		assert.Equal(t, want, ids(ForProfile(corpus, profileID)), "under profile %q", profileID)
	}
	assert.NotNil(t, ForProfile(nil, "alpha"), "an empty corpus keeps an empty list, never nil")
}
