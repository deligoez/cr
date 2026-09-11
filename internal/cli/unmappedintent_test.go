package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/state"
)

// intentFinding is an intent-axis record written as a finding on u1, citing
// lib.go's first line: a location outside the unit's lines 3 to 7 that the head
// resolves, so §6.2 grades it `cited` and §6.3 has nothing to force.
func intentFinding(id string) map[string]any {
	return map[string]any{
		"id": id, "kind": "finding", "role": "intent-coverage", "class": "requirement-unmet",
		"severity": "high", "unit": "u1", "claim": fixtureIssue + "#c1",
		"anchor": map[string]any{
			"path": "lib.go", "side": "RIGHT", "start_line": 4, "line": 4,
			"content_hash": "0123456789abcdef",
		},
		"citations": []map[string]any{{"path": "lib.go", "line": 1}},
		"summary":   "Load does not do what the issue asks of it.",
		"evidence":  "The issue asks Load to return an error.",
	}
}

// §4.1.4 through `cr record`: an intent-axis finding on a unit the round's
// mapping maps to no claim never reaches findings.ndjson as kind: finding, and
// the same record on a mapped unit keeps the kind the agent wrote.
//
// Both records grade `cited`, so §6.3's forcing of an `argued` record is not
// what separates them — only the mapping is.
func TestAnIntentFindingOnAnUnmappedUnitIsStoredAsAQuestion(t *testing.T) {
	for name, mapped := range map[string]bool{"mapped": true, "unmapped": false} {
		t.Run(name, func(t *testing.T) {
			layout := detectedHome(t)
			if mapped {
				held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
				require.NoError(t, err)
				meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
				require.NoError(t, err)
				require.NoError(t, state.ReplaceStamped(held, state.FileMapping,
					state.Stamp{Head: meta.Head, Round: meta.Round},
					[]*mapping.Pair{{Claim: fixtureIssue + "#c1", Unit: "u1"}}))
				require.NoError(t, held.Unlock())
			}

			_, err := runRecord(t, fixturePR,
				writeRecordFile(t, "merged.ndjson", intentFinding("f1")), "--repo", fixtureSlug)
			require.NoError(t, err)

			stored := storedFindings(t, layout)
			require.Len(t, stored, 1)
			assert.Equal(t, "intent", stored[0].Axis)
			assert.Equal(t, finding.GradeCited, stored[0].Grade, "§6.3 forces nothing here")
			want := finding.KindQuestion
			if mapped {
				want = finding.KindFinding
			}
			assert.Equal(t, want, stored[0].Kind)
		})
	}
}
