package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The five layers of spec/0.1.0.md §2.7 in order. One key is set at every one
// of them, and each step removes the layer that just won, so the winner is
// asserted at every depth down to the built-in default.
func TestResolutionPrefersTheHighestLayer(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "config.json")
	repo := filepath.Join(dir, "repo-config.json")
	require.NoError(t, os.WriteFile(global, []byte(`{"post": {"max_comments": 10}}`), 0o600))
	require.NoError(t, os.WriteFile(repo, []byte(`{"post": {"max_comments": 15}}`), 0o600))

	src := Sources{
		Flags:        map[string]any{"post.max_comments": 30},
		Environ:      []string{"CR_POST_MAX_COMMENTS=25"},
		RepoConfig:   repo,
		GlobalConfig: global,
	}

	steps := []struct {
		layer  string
		remove func(*Sources)
		want   int
	}{
		{"command-line flags", func(*Sources) {}, 30},
		{"environment variables", func(s *Sources) { s.Flags = nil }, 25},
		{"per-repository config", func(s *Sources) { s.Environ = nil }, 15},
		{"global config", func(s *Sources) { s.RepoConfig = "" }, 10},
		{"built-in defaults", func(s *Sources) { s.GlobalConfig = "" }, 20},
	}
	for _, step := range steps {
		step.remove(&src)
		cfg, err := Resolve(src)
		require.NoError(t, err, step.layer)
		assert.Equal(t, step.want, cfg.Int("post.max_comments"), "%s must win", step.layer)
	}
}

// §2.7 keeps three decisions off the configuration surface, so a name that
// would address one is refused wherever it is written, and the refusal names
// it. CR_HOME is the state root rather than a setting, so it must survive the
// same scan untouched.
func TestProtectedNamesAreRejected(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(global, []byte(`{"post": {"auto_confirm": true}}`), 0o600))

	_, err := Resolve(Sources{GlobalConfig: global})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "post.auto_confirm")
	assert.Contains(t, err.Error(), "confirmation gate")

	for _, name := range []string{"CR_POST_CONFIRM", "CR_ARGUED_AS_FINDING", "CR_QUESTION_LABEL"} {
		_, err := Resolve(Sources{Environ: []string{name + "=1"}})
		var protected *ProtectedError
		require.ErrorAs(t, err, &protected, name)
		assert.Equal(t, name, protected.Name)
	}

	cfg, err := Resolve(Sources{Environ: []string{state.HomeEnv + "=" + dir}})
	require.NoError(t, err)
	assert.Equal(t, 20, cfg.Int("post.max_comments"))
}

// Every layer speaks a different dialect: a file supplies JSON types, the
// environment supplies strings only, and a flag supplies whatever its parser
// produced. All three must land on the setting's own type.
func TestLayerValuesAreCoercedToTheSettingType(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(global, []byte(
		`{"render": {"lang": "en"}, "ignore": {"globs": ["vendor/**"]}, "rules": {"dead_after": 12}}`,
	), 0o600))

	cfg, err := Resolve(Sources{
		GlobalConfig: global,
		Environ:      []string{`CR_INTENT_CMD=["gh","issue","view","{key}"]`},
		Flags:        map[string]any{"rules.harvest_min": 5},
	})
	require.NoError(t, err)

	assert.Equal(t, "en", cfg.String("render.lang"))
	assert.Equal(t, []string{"vendor/**"}, cfg.Strings("ignore.globs"))
	assert.Equal(t, 12, cfg.Int("rules.dead_after"))
	assert.Equal(t, []string{"gh", "issue", "view", "{key}"}, cfg.Strings("intent.cmd"))
	assert.Equal(t, 5, cfg.Int("rules.harvest_min"))

	_, err = Resolve(Sources{Environ: []string{"CR_POST_MAX_COMMENTS=many"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "post.max_comments")
}

// CR_HOME is the state root of §2.2 rather than a setting, so §2.7's scan lets
// it through. That exemption cannot widen by accident on two counts: it is
// equality against one constant, and it grants no immunity at all — the state
// root passes the scan on its own spelling, so deleting the skip would change
// nothing. Every neighbouring name is scanned as usual.
func TestOnlyTheStateRootIsExemptFromTheProtectedScan(t *testing.T) {
	dir := t.TempDir()

	cfg, err := Resolve(Sources{Environ: []string{state.HomeEnv + "=" + dir}})
	require.NoError(t, err)
	assert.Equal(t, 20, cfg.Int("post.max_comments"))
	assert.NotContains(t, cfg.Map(), "home", "the state root is not a setting")

	require.NoError(t, checkProtected(state.HomeEnv, strings.TrimPrefix(state.HomeEnv, EnvPrefix)),
		"the state root must survive the scan without the exemption")

	for _, name := range []string{
		state.HomeEnv + "_LABEL",
		state.HomeEnv + "_CONFIRM",
		state.HomeEnv + "CONFIRM",
		state.HomeEnv + "_EVIDENCE_REGION",
	} {
		_, err := Resolve(Sources{Environ: []string{name + "=" + dir}})
		var protected *ProtectedError
		require.ErrorAs(t, err, &protected, name)
		assert.Equal(t, name, protected.Name)
	}
}
