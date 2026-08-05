package config

import (
	"os"
	"path/filepath"
	"testing"

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
