package cli

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// rewriteStoredProposals hands every line of proposals.ndjson to change and
// writes the lines back, standing in for a proposal that reached the store by
// another road than today's `cr proposals record`.
func rewriteStoredProposals(t *testing.T, l state.Layout, change func(map[string]any)) {
	t.Helper()
	stored := storedRecords(t, l, state.FileProposals)
	var lines strings.Builder
	for _, line := range stored {
		change(line)
		encoded, err := json.Marshal(line)
		require.NoError(t, err)
		lines.Write(encoded)
		lines.WriteString("\n")
	}
	require.NoError(t, os.WriteFile(l.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileProposals),
		[]byte(lines.String()), 0o600))
}

// §5.7.3: a proposal of another round, or of another head, is refused with the
// state code, naming which of the two it is, and runs nothing.
func TestAProposalOfAnotherRoundOrHeadIsRefused(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	at := briefedForProposals(t, prepared)
	_, err := runCLIPrinting(t, "proposals", "record", fixturePR, proposalFile(t, at, ""), "--repo", fixtureSlug)
	require.NoError(t, err)
	meta, err := prepared.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	round := strconv.Itoa(meta.Round)
	const elsewhere = "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c"

	for name, tc := range map[string]struct {
		round float64
		head  string
		want  string
	}{
		"another round": {float64(meta.Round - 1), meta.Head, "proposal " + at.FirstP + " was proposed in round " +
			strconv.Itoa(meta.Round-1) + " and this pull request is in round " + round +
			"; §5.7.3 runs a proposal of the current round alone"},
		"another head": {float64(meta.Round), elsewhere, "proposal " + at.FirstP + " was proposed at head " +
			elsewhere + " and the round is at head " + meta.Head + "; §5.7.3 runs a proposal of the current head alone"},
	} {
		t.Run(name, func(t *testing.T) {
			rewriteStoredProposals(t, prepared, func(line map[string]any) {
				line["round"], line["head"] = tc.round, tc.head
			})

			err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug, "--proposal", at.FirstP)

			require.Error(t, err)
			assert.Equal(t, tc.want, err.Error())
			assert.Equal(t, ExitState, exitCodeFor(err))
			assert.Empty(t, storedRecords(t, prepared, state.FileProbes), "a refused proposal writes no probe")
		})
	}
}
