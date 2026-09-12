package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cr config prints the configuration that is actually in force, so the layers
// of spec/0.1.0.md §2.7 are inspectable rather than inferred.
func TestConfigCommandPrintsTheEffectiveConfiguration(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".cr")
	t.Setenv(state.HomeEnv, root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "repos", "acme", "web"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"),
		[]byte(`{"post": {"max_comments": 9}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "repos", "acme", "web", "config.json"),
		[]byte(`{"post": {"max_comments": 7}}`), 0o600))

	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"config", "--repo", "acme/web"})
	require.NoError(t, cmd.Execute())

	var printed map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &printed))
	assert.Equal(t, float64(7), printed["post.max_comments"])
	assert.Equal(t, "tr", printed["render.lang"])
	assert.Equal(t, []any{}, printed["ignore.globs"])
}

// resolvedConfig runs `cr config --resolved` for the fixture's repository and
// returns the annotated document.
func resolvedConfig(t *testing.T) map[string]resolvedSetting {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "--resolved", "--repo", "acme/web"})
	require.NoError(t, cmd.Execute())
	var printed map[string]resolvedSetting
	require.NoError(t, json.Unmarshal(out.Bytes(), &printed))
	return printed
}

// §2.7: `cr config --resolved` annotates every setting with the layer that
// supplied it, and the annotation names the file or the variable rather than
// the layer's rank alone.
//
// One key is carried up the layers and the annotation is read after each move,
// which is the only way to see that it is derived rather than guessed. A
// resolution that annotated by asking which file happens to hold the key today
// would be right about the first two steps and wrong about the third, where two
// files and a variable all name it; one that annotated by rank alone would be
// right about every step and would still leave the reader without the path to
// open — and with two config files in play, the rank does not say which.
//
// The settings nobody touched are asserted in the same run. The built-in
// defaults are the commonest answer by far, and a listing that annotated only
// what a layer overrode would leave a reader inferring the default from an
// absence.
func TestTheResolvedAnnotationFollowsAKeyBetweenLayers(t *testing.T) {
	root := crHome(t)
	repoDir := filepath.Join(root, "repos", "acme", "web")
	require.NoError(t, os.MkdirAll(repoDir, 0o700))
	global := filepath.Join(root, "config.json")
	perRepo := filepath.Join(repoDir, "config.json")

	// The global config alone: the layer is §2.7's fourth, and the source
	// is the file a reader opens.
	require.NoError(t, os.WriteFile(global, []byte(`{"post": {"max_comments": 9}}`), 0o600))
	fromGlobal := resolvedConfig(t)
	assert.Equal(t, resolvedSetting{
		Value: float64(9), From: "global config", Source: global,
	}, fromGlobal["post.max_comments"])
	assert.Equal(t, resolvedSetting{Value: float64(12), From: "built-in default"},
		fromGlobal["cluster.gap_lines"], "§2.7's lowest layer is an answer, not an absence")

	// The same key in the per-repository file: §2.7's third layer wins,
	// and the annotation moves with it although the global file still
	// names it.
	require.NoError(t, os.WriteFile(perRepo, []byte(`{"post": {"max_comments": 7}}`), 0o600))
	fromRepo := resolvedConfig(t)
	assert.Equal(t, resolvedSetting{
		Value: float64(7), From: "per-repository config", Source: perRepo,
	}, fromRepo["post.max_comments"])

	// And in the environment: §2.7's second layer wins over both files,
	// and the source is the variable to unset rather than a path.
	t.Setenv("CR_POST_MAX_COMMENTS", "5")
	fromEnv := resolvedConfig(t)
	assert.Equal(t, resolvedSetting{
		Value: float64(5), From: "environment", Source: "CR_POST_MAX_COMMENTS",
	}, fromEnv["post.max_comments"])
}

// The annotated listing and the plain one report the same values, for every
// setting.
//
// They are two readings of one resolution, and a reader who runs both must not
// be told two things. The failure this refuses is a `--resolved` that resolved
// the layers a second time: it would agree on almost every run and disagree on
// the one where something changed between the two reads.
func TestTheAnnotatedListingReportsThePlainListingsValues(t *testing.T) {
	root := crHome(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "repos", "acme", "web"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"),
		[]byte(`{"post": {"max_comments": 9}, "ignore": {"globs": ["vendor/**"]}}`), 0o600))

	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "--repo", "acme/web"})
	require.NoError(t, cmd.Execute())
	var plain map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &plain))

	annotated := resolvedConfig(t)

	require.Len(t, annotated, len(plain), "every setting the plain listing prints is annotated")
	for key, value := range plain {
		assert.Equal(t, value, annotated[key].Value, "%s", key)
		assert.NotEmpty(t, annotated[key].From, "%s: §2.7 names a layer for every setting", key)
	}
}
