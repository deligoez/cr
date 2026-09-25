package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/render"
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
		{"post.force_findings", "CR_POST_FORCE_FINDINGS", "the argued forcing of §6.3"},
		{"render.question_label", "CR_RENDER_QUESTION_LABEL", "the question label of §8.1.4"},
		{"render.provenance_region", "CR_RENDER_PROVENANCE_REGION", "the provenance region of §8.1.6"},
		{"render.evidence_region", "CR_RENDER_EVIDENCE_REGION", "the evidence region of §8.1.7"},
	}

	for _, name := range protectedNames {
		dir := t.TempDir()
		global := filepath.Join(dir, "config.json")
		repo := filepath.Join(dir, "repo-config.json")
		section, leaf, _ := strings.Cut(name.key, ".")
		body := fmt.Appendf(nil, "{%q: {%q: true}}", section, leaf)
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

// `force` names §6.3's argued forcing as plainly as `forcing` does, so a name
// carrying it is refused — and the three words that embed its letters without
// naming the decision are driven through every layer that scans and come out
// unrefused, because a carrier missing from the list would refuse them all.
func TestForceIsProtectedAndItsCarriersAreNot(t *testing.T) {
	for _, name := range []string{"CR_POST_FORCE_FINDINGS", "CR_REVIEW_FORCE"} {
		_, err := Resolve(Sources{Environ: []string{name + "=1"}})
		var protected *ProtectedError
		require.ErrorAs(t, err, &protected, name)
		assert.Equal(t, "the argued forcing of §6.3", protected.Subject)
	}

	for _, carrier := range []struct{ key, env string }{
		{"rules.enforce_min", "CR_RULES_ENFORCE_MIN"},
		{"intent.reinforce", "CR_INTENT_REINFORCE"},
		{"stats.workforce", "CR_STATS_WORKFORCE"},
	} {
		file := writeConfig(t, carrier.key)
		for layer, sources := range map[string]Sources{
			"environment variable":  {Environ: []string{carrier.env + "=1"}},
			"global config":         {GlobalConfig: file},
			"per-repository config": {RepoConfig: file},
		} {
			_, err := Resolve(sources)
			require.NoError(t, err, "%s from the %s", carrier.key, layer)
		}
	}
}

// Flags are exempt by design: §8.5.2 requires --confirm to exist as a flag. The
// exemption is scoped to that layer and reaches nothing, because a flag entry is
// keyed by setting key and no built-in setting addresses a protected decision.
// A protected name handed in as a flag therefore configures nothing, while the
// same name from the environment or from a file is still refused.
func TestNoFlagLayerCanIntroduceAProtectedSetting(t *testing.T) {
	for _, s := range settings {
		require.NoError(t, checkProtected(s.key, s.key),
			"a built-in setting must not address a protected decision")
	}

	cfg, err := Resolve(Sources{Flags: map[string]any{
		"post.auto_confirm":     true,
		"render.question_label": "asked",
	}})
	require.NoError(t, err)
	assert.NotContains(t, cfg.Map(), "post.auto_confirm")
	assert.NotContains(t, cfg.Map(), "render.question_label")

	var protected *ProtectedError
	_, err = Resolve(Sources{Environ: []string{"CR_POST_AUTO_CONFIRM=1"}})
	require.ErrorAs(t, err, &protected)
	_, err = Resolve(Sources{GlobalConfig: writeConfig(t, "post.auto_confirm")})
	require.ErrorAs(t, err, &protected)
}

// writeConfig writes a config file carrying one key, spelled exactly as given.
func writeConfig(t *testing.T, key string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, fmt.Appendf(nil, "{%q: true}", key), 0o600))
	return path
}

// §8.1.1 defaults render.lang to en, and round 8's finding
// render-lang-domain-unbounded closes its domain at the two languages §8.1.4
// builds a question label in. A layer supplying anything else is refused at
// resolution, naming the setting: a language cr has no label for leaves §6.3's
// forcing with nothing to reach the reader through, and the run that would find
// that out is the run that is about to post.
//
// Every layer that can supply the value is exercised, because the check runs
// once after they have all settled, and a refusal proved through one of them
// says nothing about the other three.
func TestRenderLangDefaultsToEnglishAndRejectsAnUnknownLanguage(t *testing.T) {
	defaults, err := Resolve(Sources{})
	require.NoError(t, err)
	assert.Equal(t, render.LangEN.String(), defaults.String(render.Setting),
		"§8.1.1 defaults render.lang to en")

	for _, lang := range render.Langs() {
		cfg, err := Resolve(Sources{Environ: []string{"CR_RENDER_LANG=" + lang.String()}})
		require.NoErrorf(t, err, "%s is enumerated and must resolve", lang)
		assert.Equal(t, lang.String(), cfg.String(render.Setting))
	}

	dir := t.TempDir()
	global := filepath.Join(dir, "config.json")
	repo := filepath.Join(dir, "repo-config.json")
	body := []byte(`{"render": {"lang": "de"}}`)
	require.NoError(t, os.WriteFile(global, body, 0o600))
	require.NoError(t, os.WriteFile(repo, body, 0o600))

	for layer, sources := range map[string]Sources{
		"command-line flags":    {Flags: map[string]any{render.Setting: "de"}},
		"environment variables": {Environ: []string{"CR_RENDER_LANG=de"}},
		"per-repository config": {RepoConfig: repo},
		"global config":         {GlobalConfig: global},
	} {
		t.Run(layer, func(t *testing.T) {
			_, err := Resolve(sources)
			var unknown *render.UnknownLangError
			require.ErrorAs(t, err, &unknown)
			assert.Equal(t, "de", unknown.Value)
			assert.Contains(t, err.Error(), render.Setting, "the refusal names the setting to edit")
		})
	}
}

// §2.7's scan reads every key a file carries, at every depth. A key nested
// three objects deep flattens to two dots and no underscore, and depth is not
// an escape: a `confirm` under two parents addresses the confirmation gate
// exactly as `post.confirm` does.
//
// gremlins found the other half. The capacity sizing `words` was annotated as
// unobservable, and the annotation named the wrong `+`: with the first one
// turned into `-`, `strings.Count(name, "_") - strings.Count(name, ".") + 1` is
// -1 for `a.b.c`, and `make` panics with `makeslice: cap out of range` rather
// than allocating a smaller slice. Every key the other fixtures reach carries
// at most one dot, which is where the two spellings agree.
func TestTheProtectedScanReadsKeysAtEveryDepth(t *testing.T) {
	unknown := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(unknown, []byte(`{"a": {"b": {"c": true}}}`), 0o600))

	cfg, err := Resolve(Sources{GlobalConfig: unknown})
	require.NoError(t, err)
	assert.NotContains(t, cfg.Map(), "a.b.c", "an unknown key configures nothing")

	protectedFile := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(protectedFile, []byte(`{"post": {"gate": {"seconds": 1}}}`), 0o600))

	_, err = Resolve(Sources{GlobalConfig: protectedFile})
	var protected *ProtectedError
	require.ErrorAs(t, err, &protected)
	assert.Equal(t, "post.gate.seconds", protected.Name)
}

// `profile` is joined into a profile file's path, so a value that is not one
// file stem is refused naming the layer, and one that is resolves as given.
func TestAProfileThatIsNotOneFileStemIsRefusedNamingTheLayer(t *testing.T) {
	resolved, err := Resolve(Sources{Flags: map[string]any{profileSetting: "shop"}})
	require.NoError(t, err)
	assert.Equal(t, "shop", resolved.String(profileSetting))

	_, err = Resolve(Sources{Flags: map[string]any{profileSetting: "../shop"}})

	var refused *LayerError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, profileSetting, refused.Key)
	assert.Contains(t, err.Error(), "../shop")
}
