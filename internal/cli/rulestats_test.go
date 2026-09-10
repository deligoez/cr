package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
)

// ledgerLines reads the fixture repository's rule-stats.ndjson as the objects
// its lines hold, off the bytes rather than through rule.Stat, so a field the
// type carries and the file does not would show here as missing.
func ledgerLines(t *testing.T, layout state.Layout) []map[string]any {
	t.Helper()
	body, err := os.ReadFile(layout.RepoRuleStats(fixtureOwner, fixtureProject))
	require.NoError(t, err)
	lines := make([]map[string]any, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		lines = append(lines, entry)
	}
	return lines
}

// §2.6.1.6 through the command, and the criterion round 9 gave it: detection
// run twice in one round leaves the ledger holding one entry per hit, not two,
// and every entry carries the rule id, the matched path and line, the round,
// the head, the pull request, and the moment in UTC.
func TestRulesCheckRunTwiceInOneRoundLeavesOneLedgerEntryPerHit(t *testing.T) {
	layout := detectedHome(t)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)

	runRulesCheck(t)
	runRulesCheck(t)

	lines := ledgerLines(t, layout)
	require.Len(t, lines, 3, "three hits, detected twice, are three entries")
	for at, entry := range lines {
		assert.Equal(t, "no-panic", entry["rule"])
		assert.Equal(t, "lib.go", entry["path"])
		assert.InDelta(t, float64(4+at), entry["line"], 0)
		assert.Equal(t, "hit", entry["event"])
		assert.InDelta(t, float64(fixturePRNumber), entry["pr"], 0)
		assert.InDelta(t, float64(meta.Round), entry["round"], 0)
		assert.Equal(t, meta.Head, entry["head"])
		stamped, isString := entry["at"].(string)
		require.True(t, isString)
		assert.True(t, strings.HasSuffix(stamped, "Z"), "the timestamp %s is written in UTC", stamped)
	}
}

// Round 9's rule-stats-event-producer through the command: `cr record` writes a
// record event for the record it stored, at that record's anchor, and a
// dismissal for each of the two hits no record of the round confirms.
func TestRecordWritesTheRecordAndDismissesTheHitsNoRecordConfirms(t *testing.T) {
	layout := detectedHome(t)
	runRulesCheck(t)

	_, err := runRecord(t, fixturePR,
		writeRecordFile(t, "merged.ndjson", confirming("f1", 4)), "--repo", fixtureSlug)
	require.NoError(t, err)

	held, err := rule.ReadStats(layout, fixtureOwner, fixtureProject)
	require.NoError(t, err)
	byEvent := map[rule.Event][]int{}
	for _, entry := range held {
		byEvent[entry.Event] = append(byEvent[entry.Event], entry.Line)
		if entry.Event == rule.EventRecord {
			assert.Equal(t, "f1", entry.Record)
		}
	}
	assert.Equal(t, map[rule.Event][]int{
		rule.EventHit:       {4, 5, 6},
		rule.EventRecord:    {4},
		rule.EventDismissal: {5, 6},
	}, byEvent)
}
