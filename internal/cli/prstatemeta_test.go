package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// storedMeta is meta.json as its keys stand on disk.
func storedMeta(t *testing.T) map[string]any {
	t.Helper()
	body, err := os.ReadFile(state.New(crHomeOf(t)).PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileMeta))
	require.NoError(t, err)
	var recorded map[string]any
	require.NoError(t, json.Unmarshal(body, &recorded))
	return recorded
}

// §3.7.1: `cr brief` records the state of a pull request that is not open in
// meta.json, with the time GitHub reports, for §10.4.10 to read; an open one
// records neither key.
func TestBriefRecordsTheStateOfAPullRequestThatIsNotOpen(t *testing.T) {
	for prState, want := range map[string]string{gh.StateMerged: "merged", gh.StateClosed: "closed"} {
		t.Run(prState, func(t *testing.T) {
			stateHome(t, prState)
			recorded := storedMeta(t)
			assert.Equal(t, want, recorded["pr_state"])
			assert.Equal(t, closedAt, recorded["pr_state_at"])
		})
	}
	t.Run("OPEN", func(t *testing.T) {
		stateHome(t, "OPEN")
		recorded := storedMeta(t)
		assert.NotContains(t, recorded, "pr_state")
		assert.NotContains(t, recorded, "pr_state_at")
	})
}
