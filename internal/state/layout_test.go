package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every path of spec/0.1.0.md §2.2 comes from the layout, so no command has a
// reason to join path segments of its own.
func TestLayoutDerivesEveryStatePath(t *testing.T) {
	root := filepath.Join("home", ".cr")
	l := New(root)

	cases := map[string]struct{ got, want string }{
		"root":            {l.Root(), root},
		"config":          {l.Config(), filepath.Join(root, "config.json")},
		"profiles dir":    {l.ProfilesDir(), filepath.Join(root, "profiles")},
		"profile":         {l.Profile("generic"), filepath.Join(root, "profiles", "generic.json")},
		"roles dir":       {l.RolesDir(), filepath.Join(root, "roles")},
		"role":            {l.Role("correctness"), filepath.Join(root, "roles", "correctness.json")},
		"rules dir":       {l.RulesDir(), filepath.Join(root, "rules")},
		"rule":            {l.Rule("no-facade"), filepath.Join(root, "rules", "no-facade.json")},
		"repos dir":       {l.ReposDir(), filepath.Join(root, "repos")},
		"repo dir":        {l.RepoDir("acme", "web"), filepath.Join(root, "repos", "acme", "web")},
		"repo config":     {l.RepoConfig("acme", "web"), filepath.Join(root, "repos", "acme", "web", "config.json")},
		"repo roles dir":  {l.RepoRolesDir("acme", "web"), filepath.Join(root, "repos", "acme", "web", "roles")},
		"repo role":       {l.RepoRole("acme", "web", "convention"), filepath.Join(root, "repos", "acme", "web", "roles", "convention.json")},
		"repo rules dir":  {l.RepoRulesDir("acme", "web"), filepath.Join(root, "repos", "acme", "web", "rules")},
		"repo rule":       {l.RepoRule("acme", "web", "no-facade"), filepath.Join(root, "repos", "acme", "web", "rules", "no-facade.json")},
		"repo triage":     {l.RepoTriage("acme", "web"), filepath.Join(root, "repos", "acme", "web", "triage.ndjson")},
		"repo rule stats": {l.RepoRuleStats("acme", "web"), filepath.Join(root, "repos", "acme", "web", "rule-stats.ndjson")},
		"state dir":       {l.StateDir(), filepath.Join(root, "state")},
		"repo state dir":  {l.RepoStateDir("acme", "web"), filepath.Join(root, "state", "acme", "web")},
		"pr dir":          {l.PRDir("acme", "web", 42), filepath.Join(root, "state", "acme", "web", "pr-42")},
		"context dir":     {l.ContextDir(), filepath.Join(root, "context")},
		"context file":    {l.ContextFile("ACME-7"), filepath.Join(root, "context", "ACME-7.ndjson")},
		"waivers dir":     {l.WaiversDir(), filepath.Join(root, "waivers")},
		"waivers file":    {l.WaiversFile("acme", "web"), filepath.Join(root, "waivers", "acme", "web.ndjson")},
		"locks dir":       {l.LocksDir(), filepath.Join(root, "locks")},
		"pr lock file":    {l.PRLockFile("acme", "web", 42), filepath.Join(root, "locks", "acme", "web", "pr-42.lock")},
	}
	for name, c := range cases {
		assert.Equal(t, c.want, c.got, name)
	}
}

// The root is injectable, so the suite never writes into a real ~/.cr.
func TestDefaultPrefersTheHomeEnvOverride(t *testing.T) {
	t.Setenv(HomeEnv, filepath.Join("tmp", "cr-state"))
	l, err := Default()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("tmp", "cr-state"), l.Root())

	t.Setenv(HomeEnv, "")
	l, err = Default()
	require.NoError(t, err)
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, dirName), l.Root())
}

func TestInitCreatesTheGlobalTree(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, l.Init())

	for _, dir := range []string{
		l.Root(), l.ProfilesDir(), l.RolesDir(), l.RulesDir(), l.ReposDir(),
		l.StateDir(), l.ContextDir(), l.WaiversDir(), l.LocksDir(),
	} {
		info, err := os.Stat(dir)
		require.NoError(t, err, dir)
		assert.True(t, info.IsDir(), dir)
	}

	body, err := os.ReadFile(l.Config())
	require.NoError(t, err)
	assert.Equal(t, emptyConfig, string(body))
}

// A second init must never discard what the first one left behind.
func TestInitIsIdempotent(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, l.Init())
	require.NoError(t, os.WriteFile(l.Config(), []byte(`{"profile":"generic"}`), filePerm))

	require.NoError(t, l.Init())

	body, err := os.ReadFile(l.Config())
	require.NoError(t, err)
	assert.Equal(t, `{"profile":"generic"}`, string(body))
}

func TestEnsureCreatesTheScopedPaths(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, l.Init())
	require.NoError(t, l.EnsurePR("acme", "web", 42))
	require.NoError(t, l.EnsureContext("ACME-7"))

	for _, dir := range []string{
		l.RepoDir("acme", "web"), l.RepoRolesDir("acme", "web"),
		l.RepoRulesDir("acme", "web"), l.PRDir("acme", "web", 42),
	} {
		info, err := os.Stat(dir)
		require.NoError(t, err, dir)
		assert.True(t, info.IsDir(), dir)
	}
	for _, file := range []string{
		l.RepoConfig("acme", "web"), l.RepoTriage("acme", "web"),
		l.RepoRuleStats("acme", "web"), l.WaiversFile("acme", "web"),
		l.ContextFile("ACME-7"),
	} {
		info, err := os.Stat(file)
		require.NoError(t, err, file)
		assert.False(t, info.IsDir(), file)
	}

	require.NoError(t, os.WriteFile(l.RepoTriage("acme", "web"), []byte("{}\n"), filePerm))
	require.NoError(t, l.EnsurePR("acme", "web", 42))
	body, err := os.ReadFile(l.RepoTriage("acme", "web"))
	require.NoError(t, err)
	assert.Equal(t, "{}\n", string(body))
}
