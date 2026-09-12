package cli

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
)

// listedRuleJSON is a rule file whose id and title are the only fields that
// vary between the fixtures below.
func listedRuleJSON(id string) string {
	return `{"id":"` + id + `","title":"The ` + id + ` standard.",` +
		`"rationale":"Why ` + id + ` exists.","class":"` + id + `-class"}`
}

// layeredHome is a state root holding one rule in each of §2.6 item 1's three
// layers — no-panic per-repository, no-todo globally, and no-sleep in the
// profile the checkout's go.mod selects — plus a global no-panic the
// per-repository one shadows, and a checkout for §2.4.1 to select in.
func layeredHome(t *testing.T) state.Layout {
	t.Helper()
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	repoRules := layout.RepoRulesDir(harvestOwner, harvestRepo)
	require.NoError(t, os.MkdirAll(repoRules, 0o750))
	require.NoError(t, os.WriteFile(repoRules+"/no-panic.json", []byte(listedRuleJSON("no-panic")), 0o600))
	require.NoError(t, os.WriteFile(layout.Rule("no-panic"), []byte(listedRuleJSON("no-panic")), 0o600))
	require.NoError(t, os.WriteFile(layout.Rule("no-todo"), []byte(listedRuleJSON("no-todo")), 0o600))
	require.NoError(t, layout.EnsureProfile("gomod", `{"id":"gomod","match":{"files":["go.mod"],"globs":["**/*.go"]},`+
		`"axes":{"intent":true,"correctness":true,"convention":true,"test":true},`+
		`"rules":[`+listedRuleJSON("no-sleep")+`]}`))

	checkout := t.TempDir()
	require.NoError(t, os.WriteFile(checkout+"/go.mod", []byte("module example.com/api\n"), 0o600))
	restore := repoDir
	repoDir = func() (string, error) { return checkout, nil }
	t.Cleanup(func() { repoDir = restore })
	return layout
}

// listRules runs `cr rules list` with args and decodes what it printed into v.
func listRules(t *testing.T, v any, args ...string) {
	t.Helper()
	printed, err := runCLIPrinting(t, append([]string{"rules", "list", "--repo", harvestSlug}, args...)...)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(printed), v))
}

// layersOf is a listing as id and layer, which is §11's "the layer each came
// from".
func layersOf(listed []listedRule) map[string]string {
	layers := make(map[string]string, len(listed))
	for _, r := range listed {
		layers[r.ID] = r.From
	}
	return layers
}

// §11: `cr rules list` without --dead reports the effective rules with the layer
// each came from, and a per-repository rule shadowing a global one of the same
// id is reported once, at the layer that won it (§2.6 item 2).
func TestRulesListReportsTheEffectiveRulesWithTheirLayer(t *testing.T) {
	layeredHome(t)

	var printed rulesListResult
	listRules(t, &printed)

	assert.Equal(t, "gomod", printed.Profile)
	assert.Equal(t, map[string]string{"no-panic": "repo", "no-todo": "global", "no-sleep": "profile"},
		layersOf(printed.Rules))
	require.Len(t, printed.Rules, 3, "§2.6 item 2: the shadowed global no-panic is not a second entry")
	assert.Contains(t, printed.Rules[0].Path, "repos", "the winning no-panic is the per-repository file")
	assert.Empty(t, printed.Honesty)
}

// hitAt records one hit of ruleID on pull request pr's round, written at minute
// past a fixed hour, through the ledger's one writer.
func hitAt(t *testing.T, l state.Layout, ruleID string, pr, round, minute int) {
	t.Helper()
	require.NoError(t, rule.RecordHits(l, harvestOwner, harvestRepo,
		[]rule.Hit{{RuleID: ruleID, Path: "lib.go", Line: 4}},
		&rule.Occasion{
			PR: pr, Round: round, Head: harvestHead,
			At: time.Date(2026, 9, 1, 10, minute, 0, 0, time.UTC),
		}))
}

// §2.6.3.4 and round 8's cross-pr-round-ordering-undefined: the window is the
// last rules.dead_after distinct (pull request, round) pairs ordered by the
// ledger's timestamp, so it spans both pull requests when their rounds
// interleave in time.
//
// Pull request 7's round 1 and pull request 12's round 1 are the two oldest
// pairs. With rules.dead_after at 4 the window is PR 12 round 3, PR 7 round 2,
// PR 12 round 2 and PR 7 round 3 by timestamp. no-todo hit only in PR 12 round
// 2, inside the window, and no-panic only in PR 7 round 1, outside it — a
// window counted per pull request, or by round index alone, would reach PR 7
// round 1 and keep no-panic alive.
func TestTheDeadWindowSpansTwoPullRequestsByTimestamp(t *testing.T) {
	layout := layeredHome(t)
	t.Setenv("CR_RULES_DEAD_AFTER", "4")
	hitAt(t, layout, "no-panic", 7, 1, 0)
	hitAt(t, layout, "no-sleep", 12, 1, 1)
	hitAt(t, layout, "other", 7, 3, 2)
	hitAt(t, layout, "no-todo", 12, 2, 3)
	hitAt(t, layout, "other", 7, 2, 4)
	hitAt(t, layout, "other", 12, 3, 5)

	var printed rulesDeadResult
	listRules(t, &printed, "--dead")

	assert.Equal(t, 4, printed.DeadAfter)
	assert.True(t, printed.Full)
	pairs := make([][2]int, 0, len(printed.Window))
	for _, pair := range printed.Window {
		pairs = append(pairs, [2]int{pair.PR, pair.Round})
	}
	assert.Equal(t, [][2]int{{12, 3}, {7, 2}, {12, 2}, {7, 3}}, pairs,
		"the window is ordered by the ledger's timestamp across both pull requests")
	assert.Equal(t, map[string]string{"no-panic": "repo", "no-sleep": "profile"}, layersOf(printed.Dead),
		"no-todo hit inside the window; no-panic and no-sleep hit only before it")
}

// A ledger holding fewer rounds than rules.dead_after calls no rule dead, and
// says why: "no hit across the last twenty rounds" is not something three
// rounds can establish.
func TestAShortLedgerCallsNoRuleDead(t *testing.T) {
	layout := layeredHome(t)
	hitAt(t, layout, "no-todo", 7, 1, 0)

	var printed rulesDeadResult
	listRules(t, &printed, "--dead")

	assert.Equal(t, 20, printed.DeadAfter, "§2.6.3.4's default is 20")
	assert.False(t, printed.Full)
	assert.Empty(t, printed.Dead)
	require.Len(t, printed.Honesty, 1)
	assert.Contains(t, printed.Honesty[0], "no rule can be called dead yet")
}
