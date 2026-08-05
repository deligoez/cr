package state

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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
	}
	for name, c := range cases {
		assert.Equal(t, c.want, c.got, name)
	}
}
