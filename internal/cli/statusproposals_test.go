package cli

import (
	"encoding/json"
	"maps"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/proposal"
	"github.com/deligoez/cr/internal/state"
)

// §10.1.8 over every state §5.7 gives a proposal: one already run is counted
// run and not open, and one §5.7.5 stored unrunnable is counted and named with
// its reason, so the round's total is the three states' sum.
func TestStatusCountsRunAndUnrunnableProposals(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	at := briefedForProposals(t, prepared)
	_, err := runCLIPrinting(t, "proposals", "record", fixturePR, proposalFile(t, at, ""), "--repo", fixtureSlug)
	require.NoError(t, err)
	_, err = runCLIPrinting(t, "probe", "run", fixturePR, "--repo", fixtureSlug, "--proposal", at.FirstP)
	require.NoError(t, err)
	first, ok := proposal.IDSuffix(at.FirstP)
	require.True(t, ok)
	second := proposal.IDOf(first + 1)
	const reason = "the target app.go:3 no longer resolves at the current head"
	ran := storedRecords(t, prepared, state.FileProposals)
	require.Len(t, ran, 1)
	unrunnable := maps.Clone(ran[0])
	delete(unrunnable, "probe")
	unrunnable["id"], unrunnable["state"], unrunnable["reason"] = second, "unrunnable", reason
	var body []byte
	for _, line := range []map[string]any{ran[0], unrunnable} {
		encoded, err := json.Marshal(line)
		require.NoError(t, err)
		body = append(append(body, encoded...), '\n')
	}
	require.NoError(t, os.WriteFile(
		prepared.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileProposals), body, 0o600))

	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)

	require.NoError(t, err)
	var report struct {
		Proposals proposalReport `json:"proposals"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	assert.Equal(t, proposalReport{
		Total: 2, Run: 1, Unrunnable: 1, OpenIDs: []string{},
		Unrunnables: []unrunnableProposal{{ID: second, Reason: reason}},
	}, report.Proposals)
}
