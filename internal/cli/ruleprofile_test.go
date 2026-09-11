package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
)

// alphaPanicRule is panicRule scoped by §2.6's `profiles` row to alpha alone.
const alphaPanicRule = `{"id":"no-panic","title":"A library returns errors rather than panicking.",` +
	`"rationale":"A panic in a library takes down every caller's process.",` +
	`"class":"panic-in-library","kind":"finding","severity":"critical","profiles":["alpha"],` +
	`"detect":{"mode":"regex","pattern":"panic\\("}}`

// scopedHome is reviewedHome with no-panic scoped to alpha, two profiles on
// disk, and the round resolved to profileID with the convention role active,
// so `cr review` has a prompt to carry the hits in.
func scopedHome(t *testing.T, profileID string) state.Layout {
	t.Helper()
	layout := reviewedHome(t)
	require.NoError(t, os.WriteFile(layout.Rule("no-panic"), []byte(alphaPanicRule), 0o600))
	for _, id := range []string{"alpha", "beta"} {
		require.NoError(t, layout.EnsureProfile(id, `{"id":"`+id+`","match":{"files":[],"globs":["**/*.go"]},`+
			`"axes":{"intent":true,"correctness":true,"convention":true,"test":true}}`))
	}
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.ProfileID, meta.ActiveRoles = profileID, []string{"convention"}
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())
	return layout
}

// §2.6's `profiles` row, asked by both commands that evaluate rules: a rule
// scoped to alpha reports no hit from `cr rules check` or `cr review` in a round
// resolved to beta, and its three hits from both in a round resolved to alpha.
//
// Each command runs in a home of its own, because both write the ledger and a
// hit one of them recorded would otherwise read as the other's.
func TestARuleScopedToAnotherProfileHitsInNeitherCommand(t *testing.T) {
	for profileID, want := range map[string]int{"alpha": 3, "beta": 0} {
		t.Run(profileID+" rules check", func(t *testing.T) {
			scopedHome(t, profileID)
			var checked rulesCheckResult
			require.NoError(t, json.Unmarshal(runRulesCheck(t), &checked))
			assert.Len(t, checked.Hits, want)
		})
		t.Run(profileID+" review", func(t *testing.T) {
			layout := scopedHome(t, profileID)
			var fan review.Fanout
			require.NoError(t, json.Unmarshal(runReview(t), &fan))
			require.Len(t, fan.Prompts, 1)
			assert.Equal(t, "convention", fan.Prompts[0].Role)
			assert.Equal(t, want, strings.Count(fan.Prompts[0].Text, "- rule no-panic at lib.go:"),
				"the hits the prompt carries")
			held, err := rule.ReadStats(layout, fixtureOwner, fixtureProject)
			require.NoError(t, err)
			assert.Len(t, held, want, "the hits cr review recorded")
		})
	}
}
