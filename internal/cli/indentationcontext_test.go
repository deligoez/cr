package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reindentingContextLine is a record whose suggestion replaces head line 1 of
// reviewedHome's lib.go, `package lib`, with a tab-indented copy of it.
//
// Line 1 is a context line: the change rewrites line 3 onward, and git's three
// lines of context put lines 1 and 2 in the hunk's head range, which is the
// range §8.2 admits a suggestion into.
func reindentingContextLine(id string) map[string]any {
	return map[string]any{
		"id": id, "kind": "finding", "role": "correctness", "class": "misplaced-package-clause",
		"severity": "high", "unit": "u1",
		"anchor": map[string]any{
			"path": "lib.go", "side": "RIGHT", "start_line": 1, "line": 1,
			"content_hash": "0123456789abcdef",
		},
		"summary":    "The package clause is indented.",
		"evidence":   "The first line of lib.go opens the file.",
		"suggestion": "\tpackage lib",
	}
}

// §8.2.3 through `cr draft` for a suggestion replacing a context line: the
// warning compares the suggestion's first line against the head line it
// replaces whether the diff changed that line or only showed it.
//
// The warning is read as the JSON document the command printed and asserted
// whole, so a run that warned about some other line, or quoted the replaced
// line from anywhere but the head, fails it.
func TestADraftWarnsAboutASuggestionReindentingAContextLine(t *testing.T) {
	reviewedHome(t)
	_, err := runRecord(t, fixturePR,
		writeRecordFile(t, "merged.ndjson", reindentingContextLine("f1")), "--repo", fixtureSlug)
	require.NoError(t, err)

	printed, err := runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err, "§8.2.3 warns; it does not refuse")

	var drafted struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &drafted), printed)
	assert.Equal(t, []string{
		"record f1: the suggestion's first line and the line it replaces are indented differently, " +
			"and §8.2.3 has cr infer nothing about indentation. Leave the block in place to post it " +
			"as written, or edit it.\n  suggestion: \"\\tpackage lib\"\n  replaces:   \"package lib\"",
	}, drafted.Warnings)
}
