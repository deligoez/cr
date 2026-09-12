package brief

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/activation"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
)

// noThreads is an empty page of review threads: this round is about the role
// set, and §3.5's ingestion has nothing to do with it.
const noThreads = `{"data":{"repository":{"pullRequest":{"reviewThreads":{` +
	`"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}}}}`

// profiled is repository() with the marker file §2.4.1 selects `laravel-pest`
// by, so a round runs with a profile resolved and every axis of §1.5 enabled.
// The fixture the other tests share carries no marker at all, which is §2.4.4's
// repository — a state where no axis runs and therefore no role is active,
// which would let a broken definition pass by returning nothing.
func profiled(t *testing.T) (dir, head, base string) {
	t.Helper()
	dir, _, base = repository(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{}\n"), 0o600))
	runGit(t, dir, "add", "composer.json")
	runGit(t, dir, "commit", "--quiet", "-m", "declare the package")
	return dir, runGit(t, dir, "rev-parse", "HEAD"), base
}

// shipped writes §2.4.5's two profiles into the state root, which is what `cr
// init` does and what profile.Select then reads.
func shipped(t *testing.T, layout state.Layout) {
	t.Helper()
	for id, body := range profile.Builtins() {
		require.NoError(t, layout.EnsureProfile(id, body))
	}
}

// §4.5.1's active set is computed and stored with `cr review` never invoked.
//
// That is round 8's `circular-definition` finding taken to the one place it can
// actually be shown: a whole round, run end to end, with no fan-out anywhere in
// it. §4.5.1's third clause reads "and §4.6.4 did not report it skipped", and
// §4.6.4 is a report `cr review` emits over this very set — so if activeness
// depended on it, meta.json's `active_roles` could not be filled here at all.
// It is, and it agrees with the definition applied directly to the same two
// inputs, which is the answer every later reader of the field gets: §4.5.6
// checks a cell's role against it and §10.2.2 counts a row of cells per active
// role, and both read this stored list rather than deriving one of their own.
func TestTheActiveRoleSetIsStoredWithoutAnyFanOut(t *testing.T) {
	dir, head, base := profiled(t)
	src := sources(t, dir, answering(head, base, noThreads))
	shipped(t, src.Layout)

	assembled, err := Run(src)
	require.NoError(t, err)
	require.Equal(t, "laravel-pest", assembled.Profile.ID, "the marker file selects the profile")

	recorded, err := src.Layout.Briefed(testOwner, testRepo, testPR,
		func() (string, error) { return assembled.Head, nil })
	require.NoError(t, err)

	// The definition, applied here to the same two inputs and nothing
	// else: the axes §4.5.1 to §4.5.3 left active, and the resolved
	// profile. No command has fanned out, and none exists to.
	selection, err := profile.Select(src.Layout.ProfilesDir(), dir, "")
	require.NoError(t, err)
	resolved, err := intent.Resolve(
		intent.KeySources{Flag: testIssue}, src.Config.String("intent.key_pattern"), src.Intent)
	require.NoError(t, err)
	corpus, err := role.Resolve(
		src.Layout.RepoRolesDir(testOwner, testRepo), src.Layout.RolesDir())
	require.NoError(t, err)
	expected := activation.Activate(&selection.Profile, resolved).
		ActiveRoles(corpus, selection.Profile.ID)

	require.Equal(t, []string{"convention", "correctness", "intent-coverage", "test-adequacy"},
		expected, "every axis runs here, so §2.5.1's four roles are all active")
	assert.Equal(t, expected, recorded.ActiveRoles,
		"§4.5.1: meta.json carries the active set the definition yields")
	assert.Equal(t, expected, assembled.ActiveRoles,
		"the payload and the file agree, so neither is a second answer")
}

// §3.7.1 reports the profile together with the layer that selected it, and the
// two layers are told apart.
//
// §2.4.1 gives a profile two ways in: automatic selection by `match.files`, and
// the per-repository `profile` setting that overrides it. §3.7.1 has the brief
// print which one applied, and the difference is what a reader acts on — a
// profile they did not expect is a marker file they did not know about under
// one layer, and a configuration line they can edit under the other.
//
// Mutation testing is why this test exists. Negating the condition swapped the
// two labels, and nothing failed: the layer was asserted only off a hand-built
// payload, never off a real selection, so no test had ever watched cr choose.
func TestTheBriefNamesWhichLayerSelectedTheProfile(t *testing.T) {
	dir, head, base := profiled(t)

	t.Run("selected by a marker file", func(t *testing.T) {
		src := sources(t, dir, answering(head, base, noThreads))
		shipped(t, src.Layout)

		assembled, err := Run(src)
		require.NoError(t, err)

		assert.Equal(t, "laravel-pest", assembled.Profile.ID)
		assert.Equal(t, LayerMarkerFiles, assembled.Profile.SelectionLayer,
			"§2.4.1: composer.json is the marker, and nothing configured a profile")
	})

	t.Run("overridden by configuration", func(t *testing.T) {
		src := sources(t, dir, answering(head, base, noThreads))
		shipped(t, src.Layout)
		resolved, err := config.Resolve(config.Sources{
			Flags: map[string]any{"profile": "generic"},
		})
		require.NoError(t, err)
		src.Config = resolved

		assembled, err := Run(src)
		require.NoError(t, err)

		assert.Equal(t, "generic", assembled.Profile.ID,
			"§2.4.1: the configured profile overrides the marker file")
		assert.Equal(t, LayerConfigured, assembled.Profile.SelectionLayer,
			"and the brief says which layer it came from")
	})
}
