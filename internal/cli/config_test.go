package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// QA D-S01-2: `cr config` inside a clone resolves the per-repository layer the
// way a pull-request command there does, detecting the repository from the
// checkout's remote when `--repo` is absent, so the value and the layer it
// prints are the ones `cr brief` in that clone reads. Before, the layer was
// consulted only under `--repo`, and a reader checking a setting inside the
// clone was shown the global value with nothing saying a layer was skipped.
//
// intent.cmd is the setting because nothing but the per-repository file sets
// it here: a listing that skipped the layer reports the built-in default, which
// no other assertion could mistake for the right answer. Outside any clone the
// listing is still printed, and says the layer was not consulted and why — the
// why being exactly what detection refused with in that directory.
func TestConfigInsideACloneConsultsTheDetectedRepositorysLayer(t *testing.T) {
	root := crHome(t)
	perRepo := filepath.Join(root, "repos", "acme", "web", "config.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(perRepo), 0o700))
	require.NoError(t, os.WriteFile(perRepo, []byte(`{"intent": {"cmd": ["tracker", "{key}"]}}`), 0o600))

	listing := func(t *testing.T) map[string]json.RawMessage {
		t.Helper()
		printed, err := runIn(t, "config", "--resolved")
		require.NoError(t, err)
		var document map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(printed), &document))
		return document
	}
	setting := func(t *testing.T, document map[string]json.RawMessage, key string) resolvedSetting {
		t.Helper()
		var decoded resolvedSetting
		require.NoError(t, json.Unmarshal(document[key], &decoded), key)
		return decoded
	}

	t.Run("inside a clone", func(t *testing.T) {
		t.Chdir(standingIn(t, "acme/web"))
		document := listing(t)
		assert.Equal(t, resolvedSetting{
			Value: []any{"tracker", "{key}"}, From: "per-repository config", Source: perRepo,
		}, setting(t, document, "intent.cmd"))
		assert.NotContains(t, document, configHonesty, "a layer that was consulted discloses nothing")
	})

	t.Run("outside any clone", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, _, refused := detectRepo()
		var undetected *RepositoryDetectionError
		require.ErrorAs(t, refused, &undetected, "the fixture has to be a directory detection refuses")

		document := listing(t)
		assert.Equal(t, "built-in default", setting(t, document, "intent.cmd").From)
		var notes []string
		require.NoError(t, json.Unmarshal(document[configHonesty], &notes))
		assert.Equal(t, []string{fmt.Sprintf(
			"the per-repository config layer of §2.7 was not consulted: %v; "+
				"run cr config inside a clone of the repository, or pass --repo <owner/repo>", undetected)},
			notes)
	})
}
