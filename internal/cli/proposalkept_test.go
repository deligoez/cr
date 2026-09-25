package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §5.7.4: a proposal whose probe came back `inconclusive` leaves the record it
// names on its earlier probe and grade, and says so; a decisive one moves it.
//
// The field report behind it, tarfin-labs/backend#6328 with cr 0.14.0: f4201
// was probed on p3, a `no-test-failed` over a passing baseline; proposal x4202
// naming it ran twice with a filter selecting no test, came back inconclusive
// both times, and left f4201 on p5, graded `argued`, its evidence gone from the
// draft. Here the first proposal settles the record on its probe, and the
// second runs twice over a suite that prints no recap.
func TestAnInconclusiveProposalLeavesTheRecordOnItsDecisiveProbe(t *testing.T) {
	inconclusive := filepath.Join(t.TempDir(), "inconclusive")
	prepared, _, _, _ := probeFixture(t,
		onlyWhenMutated("  if [ -f '"+inconclusive+"' ]; then echo 'no recap'; exit 0; fi\n"))
	at := briefedForProposals(t, prepared)
	records, recordID := recordFile(t, at)
	_, err := runCLIPrinting(t, "record", fixturePR, records, "--repo", fixtureSlug)
	require.NoError(t, err)
	number, err := strconv.Atoi(at.FirstP[1:])
	require.NoError(t, err)
	second := at.FirstP[:1] + strconv.Itoa(number+1)
	both := proposalFile(t, at, recordID)
	body, err := os.ReadFile(both)
	require.NoError(t, err)
	body = append(body, []byte(strings.Replace(string(body), mustJSON(t, at.FirstP), mustJSON(t, second), 1))...)
	require.NoError(t, os.WriteFile(both, body, 0o600))
	_, err = runCLIPrinting(t, "proposals", "record", fixturePR, both, "--repo", fixtureSlug)
	require.NoError(t, err)

	decisive := probeRunDocument(t, at.FirstP)
	require.Equal(t, "no-test-failed", decisive["result"], "the control: the first proposal settles the gap")
	held := decisive["probe"].(string)
	assert.Equal(t, map[string]any{"record": recordID, "probe": held, "was": "argued", "now": "probed"},
		decisive["regraded"], "a decisive result moves the record and re-grades it")

	require.NoError(t, os.WriteFile(inconclusive, nil, 0o600))
	for range 2 {
		ran := probeRunDocument(t, second)
		require.Equal(t, "inconclusive", ran["result"])
		probeID := ran["probe"].(string)
		assert.Equal(t, map[string]any{
			"record": recordID, "probe": probeID, "was": "probed", "now": "probed", "kept": true, "holds": held,
		}, ran["regraded"], "§5.7.4 reports that the record kept its probe and grade")
		assert.Contains(t, ran["honesty"], "record "+recordID+" keeps probe "+held+
			" and grade probed, per §5.7.4: probe "+probeID+
			" came back inconclusive, which establishes nothing, so it moves no record's probe")

		findings := storedRecords(t, prepared, state.FileFindings)
		require.Len(t, findings, 1)
		assert.Equal(t, []any{held, "probed"}, []any{findings[0]["probe"], findings[0]["grade"]},
			"the record stays on the probe that settled it")
		proposals := storedRecords(t, prepared, state.FileProposals)
		require.Len(t, proposals, 2)
		assert.Equal(t, []any{"run", probeID}, []any{proposals[1]["state"], proposals[1]["probe"]},
			"the proposal itself still moves to the probe it produced")
	}
}
