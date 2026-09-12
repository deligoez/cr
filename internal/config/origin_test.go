package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// layered writes a global and a per-repository config carrying the JSON given
// — either may be empty, which writes no file — and returns their paths.
func layered(t *testing.T, global, perRepo string) (globalPath, repoPath string) {
	t.Helper()
	dir := t.TempDir()
	globalPath = filepath.Join(dir, "config.json")
	repoPath = filepath.Join(dir, "repo-config.json")
	for path, body := range map[string]string{globalPath: global, repoPath: perRepo} {
		if body == "" {
			continue
		}
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	return globalPath, repoPath
}

// §2.7's order, read off the origins: the highest layer that supplied a key is
// the layer credited with it.
//
// The same key is set in all four layers at once, which is what separates
// "records the highest layer" from "records the last layer it happened to read".
// Both answer correctly when one layer sets a key; only the first answers
// correctly here, and the annotation is what a reader uses to find out why a
// value they can see in a file is not the value in force.
func TestTheOriginIsTheHighestLayerThatSuppliedTheKey(t *testing.T) {
	global, perRepo := layered(t,
		`{"post": {"max_comments": 9}}`, `{"post": {"max_comments": 7}}`)
	src := Sources{
		GlobalConfig: global,
		RepoConfig:   perRepo,
		Environ:      []string{"CR_POST_MAX_COMMENTS=5"},
	}

	for name, step := range map[string]struct {
		flags  map[string]any
		origin Origin
		value  int
	}{
		"the command line, §2.7's first layer": {
			flags:  map[string]any{"post.max_comments": 3},
			origin: Origin{From: LayerFlag},
			value:  3,
		},
		"the environment, its second": {
			origin: Origin{From: LayerEnv, Source: "CR_POST_MAX_COMMENTS"},
			value:  5,
		},
	} {
		t.Run(name, func(t *testing.T) {
			at := src
			at.Flags = step.flags

			resolved, err := Resolve(at)

			require.NoError(t, err)
			assert.Equal(t, step.value, resolved.Int("post.max_comments"))
			assert.Equal(t, step.origin, resolved.Origins()["post.max_comments"])
		})
	}
}

// The two file layers are told apart by name and by path, so a reader with a
// key in both files learns which one is in force.
func TestTheTwoFileLayersAreNamedApartAndByPath(t *testing.T) {
	global, perRepo := layered(t,
		`{"post": {"max_comments": 9}, "cluster": {"gap_lines": 4}}`,
		`{"post": {"max_comments": 7}}`)

	resolved, err := Resolve(Sources{GlobalConfig: global, RepoConfig: perRepo})

	require.NoError(t, err)
	origins := resolved.Origins()
	assert.Equal(t, Origin{From: LayerRepoConfig, Source: perRepo}, origins["post.max_comments"],
		"§2.7: the per-repository file outranks the global one")
	assert.Equal(t, Origin{From: LayerGlobalConfig, Source: global}, origins["cluster.gap_lines"],
		"and the key only the global file names is still the global file's")
}

// Every setting of the table carries an origin, and a key no layer touched
// carries §2.7's lowest one.
//
// The defaults are the commonest answer there is, so a map that annotated only
// what some layer overrode would leave a reader inferring the default from an
// absence — and an absence is also what a bug in the annotation looks like.
func TestEverySettingCarriesAnOriginAndAnUntouchedOneIsADefault(t *testing.T) {
	global, _ := layered(t, `{"post": {"max_comments": 9}}`, "")

	resolved, err := Resolve(Sources{GlobalConfig: global})

	require.NoError(t, err)
	origins := resolved.Origins()
	require.Len(t, origins, len(resolved.Map()), "one origin per setting, and no more")
	for key, origin := range origins {
		assert.NotEmpty(t, origin.From, "%s carries no layer", key)
		if key == "post.max_comments" {
			continue
		}
		assert.Equal(t, Origin{From: LayerDefault}, origin,
			"%s was touched by no layer, so it is a built-in default", key)
	}
}

// A key an unknown name sits beside is still the layer's, and the unknown name
// is nobody's.
//
// §2.7's table is the whole configuration surface, so a name absent from it
// configures nothing — and crediting a layer with it would put a key in the
// annotated listing that the effective configuration does not have.
func TestAnUnknownNameInALayerAnnotatesNothing(t *testing.T) {
	global, _ := layered(t, `{"post": {"max_comments": 9, "invented": 1}}`, "")

	resolved, err := Resolve(Sources{GlobalConfig: global})

	require.NoError(t, err)
	origins := resolved.Origins()
	assert.Equal(t, Origin{From: LayerGlobalConfig, Source: global}, origins["post.max_comments"],
		"the key the table knows is still the layer's")
	assert.NotContains(t, origins, "post.invented")
	assert.Len(t, origins, len(defaults), "the table is the whole surface")
}

// The origins a caller reads are a copy, so a caller cannot rewrite the
// provenance of a resolution another caller is still holding.
func TestOriginsHandsBackACopy(t *testing.T) {
	resolved, err := Resolve(Sources{})
	require.NoError(t, err)

	taken := resolved.Origins()
	taken["post.max_comments"] = Origin{From: "invented"}

	assert.Equal(t, Origin{From: LayerDefault}, resolved.Origins()["post.max_comments"])
}
