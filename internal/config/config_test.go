package config

import (
	"fmt"
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

// §2.7 keeps five decisions off the configuration surface: the confirmation
// gate, the argued forcing, the question label, and the provenance and evidence
// regions that carry the disclosure and the checkability to the author. A name
// addressing one must be refused from the environment and from both config
// files, and the refusal must name the decision it would have reached.
func TestProtectedNamesAreRejected(t *testing.T) {
	protectedNames := []struct {
		key     string
		env     string
		subject string
	}{
		{"post.auto_confirm", "CR_POST_AUTO_CONFIRM", "the confirmation gate of §8.5"},
		{"post.gate", "CR_POST_GATE", "the confirmation gate of §8.5"},
		{"review.argued_as_finding", "CR_REVIEW_ARGUED_AS_FINDING", "the argued forcing of §6.3"},
		{"review.forcing_enabled", "CR_REVIEW_FORCING_ENABLED", "the argued forcing of §6.3"},
		{"render.question_label", "CR_RENDER_QUESTION_LABEL", "the question label of §8.1.4"},
		{"render.provenance_region", "CR_RENDER_PROVENANCE_REGION", "the provenance region of §8.1.6"},
		{"render.evidence_region", "CR_RENDER_EVIDENCE_REGION", "the evidence region of §8.1.7"},
	}

	for _, name := range protectedNames {
		dir := t.TempDir()
		global := filepath.Join(dir, "config.json")
		repo := filepath.Join(dir, "repo-config.json")
		section, leaf, _ := strings.Cut(name.key, ".")
		body := []byte(fmt.Sprintf("{%q: {%q: true}}", section, leaf))
		require.NoError(t, os.WriteFile(global, body, 0o600))
		require.NoError(t, os.WriteFile(repo, body, 0o600))

		layers := []struct {
			layer   string
			sources Sources
			written string
		}{
			{"environment variable", Sources{Environ: []string{name.env + "=1"}}, name.env},
			{"global config", Sources{GlobalConfig: global}, name.key},
			{"per-repository config", Sources{RepoConfig: repo}, name.key},
		}
		for _, layer := range layers {
			_, err := Resolve(layer.sources)
			var protected *ProtectedError
			require.ErrorAs(t, err, &protected, "%s from the %s", layer.written, layer.layer)
			assert.Equal(t, layer.written, protected.Name)
			assert.Equal(t, name.subject, protected.Subject)
		}
	}
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

// §2.7 refuses a name that "would address" a protected decision, which is
// broader than an exact key and narrower than every occurrence of the letters.
// A word carrying a token addresses the decision however the name is spelled,
// while a word that merely embeds one does not. Where the two readings disagree
// the refusal wins, because being wrong costs a rename on one side and an
// implicit network write on the other.
func TestProtectedMatchingReadsWordsNotLetters(t *testing.T) {
	for _, key := range []string{
		"post.gate_timeout_seconds",
		"post.labels",
		"postconfirm",
		"render.evidenceRegion",
		"render.PROVENANCE",
	} {
		_, err := Resolve(Sources{GlobalConfig: writeConfig(t, key)})
		var protected *ProtectedError
		require.ErrorAs(t, err, &protected, key)
		assert.Equal(t, key, protected.Name)
	}

	for _, key := range []string{"rules.aggregate_min", "intent.delegate_cmd", "rules.enforcing"} {
		cfg, err := Resolve(Sources{GlobalConfig: writeConfig(t, key)})
		require.NoError(t, err, key)
		assert.NotContains(t, cfg.Map(), key, "an unknown key configures nothing")
	}
}

// writeConfig writes a config file carrying one key, spelled exactly as given.
func writeConfig(t *testing.T, key string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf("{%q: true}", key)), 0o600))
	return path
}
