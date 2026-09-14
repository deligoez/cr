package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scopedRuleJSON is listedRuleJSON scoped by §2.6's `profiles` row to the
// profiles named.
func scopedRuleJSON(id string, profiles ...string) string {
	return strings.TrimSuffix(listedRuleJSON(id), "}") + `,"profiles":["` + strings.Join(profiles, `","`) + `"]}`
}

// QA S10 rule-05 through `cr rules list`: every rule of the resolved corpus that
// the selected profile's scope leaves out is named, with its layer, its file and
// the profiles it is scoped to, instead of vanishing from the listing.
//
// layeredHome selects gomod. pest-only is a per-repository rule scoped to
// laravel-pest; no-todo is a per-repository rule scoped to laravel-pest that
// shadows the global no-todo, so §2.6 item 2 leaves one no-todo, out of profile;
// go-only is a global rule scoped to gomod and laravel-pest, so it is effective.
// With a checkout no profile matches, every scoped rule is out of profile.
func TestRulesListNamesTheRulesTheProfileScopeLeavesOut(t *testing.T) {
	layout := layeredHome(t)
	repoRules := layout.RepoRulesDir(harvestOwner, harvestRepo)
	write := func(path, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write(filepath.Join(repoRules, "pest-only.json"), scopedRuleJSON("pest-only", "laravel-pest"))
	write(filepath.Join(repoRules, "no-todo.json"), scopedRuleJSON("no-todo", "laravel-pest"))
	write(layout.Rule("go-only"), scopedRuleJSON("go-only", "gomod", "laravel-pest"))

	var printed rulesListResult
	listRules(t, &printed)

	assert.Equal(t, "gomod", printed.Profile)
	assert.Equal(t, map[string]string{"no-panic": "repo", "go-only": "global", "no-sleep": "profile"},
		layersOf(printed.Rules))
	assert.Equal(t, []scopedRule{
		{
			listedRule: listedRule{
				ID: "no-todo", Title: "The no-todo standard.", From: "repo",
				Path: filepath.Join(repoRules, "no-todo.json"),
			},
			Profiles: []string{"laravel-pest"},
		},
		{
			listedRule: listedRule{
				ID: "pest-only", Title: "The pest-only standard.", From: "repo",
				Path: filepath.Join(repoRules, "pest-only.json"),
			},
			Profiles: []string{"laravel-pest"},
		},
	}, printed.OutOfProfile)

	unmatched := t.TempDir()
	restore := repoDir
	repoDir = func() (string, error) { return unmatched, nil }
	t.Cleanup(func() { repoDir = restore })

	var none rulesListResult
	listRules(t, &none)
	assert.Empty(t, none.Profile)
	assert.Equal(t, map[string]string{"no-panic": "repo"}, layersOf(none.Rules))
	scoped := make(map[string][]string, len(none.OutOfProfile))
	for _, out := range none.OutOfProfile {
		scoped[out.ID] = out.Profiles
	}
	assert.Equal(t, map[string][]string{
		"no-todo": {"laravel-pest"}, "pest-only": {"laravel-pest"}, "go-only": {"gomod", "laravel-pest"},
	}, scoped, "no profile matched, so every scoped rule is out of profile")
}

// A corpus no `profiles` row scopes prints an empty out_of_profile list, never
// null, beside the effective rules.
func TestRulesListPrintsAnEmptyOutOfProfileListWhenNothingIsScoped(t *testing.T) {
	layeredHome(t)

	var printed map[string]any
	listRules(t, &printed)
	require.Contains(t, printed, "out_of_profile")
	assert.Equal(t, []any{}, printed["out_of_profile"])
}
