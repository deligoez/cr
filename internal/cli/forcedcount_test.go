package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// forcedOf reads the forced_to_question count out of a printed document or the
// round's summary.json, both of which carry it under that key.
func forcedOf(t *testing.T, raw []byte) finding.Forcings {
	t.Helper()
	var document struct {
		Forced finding.Forcings `json:"forced_to_question"`
	}
	require.NoError(t, json.Unmarshal(raw, &document))
	return document.Forced
}

// recordSummary is the record fixture round's summary.json, whole.
func recordSummary(t *testing.T, layout state.Layout) []byte {
	t.Helper()
	body, err := layout.ReadRound(recordOwner, recordRepo, recordPRNum, recordRound, state.FileSummary)
	require.NoError(t, err)
	return body
}

// §6.3.2 through the commands: the forced-to-question count per class counts
// the records §6.3.1 changed from finding to question, and never a record its
// role already wrote as a question.
//
// Release QA on deligoez/cr-qa#1 reported unmapped-unit 1 for f1, which the
// intent-coverage role had written as a question: the count was taken over the
// grade, and every argued record reads as a question once record time has
// forced it. So two argued records of one class go through `cr record` — one
// written as a finding, one as a question — and every place the count is
// computed must read one: the round summary and the report of `cr draft`, a
// regenerated draft, and the dry run of `cr post`.
func TestTheForcedCountOmitsAnArguedQuestionItsRoleWrote(t *testing.T) {
	layout := probedHome(t, gapFixture{result: "failed", passed: true, mapped: true, issue: gapIssue})
	standInDiff(t, []git.Hunk{{
		Path: recordPath, Side: git.Right, BaseStart: 1, BaseLines: 100, HeadStart: 1, HeadLines: 100,
	}})
	asserted := aRecord("f1", gapUnit)
	asserted["summary"] = "Does anything handle the error Decode returns?"
	asked := aRecord("f2", gapUnit)
	asked["kind"] = string(finding.KindQuestion)
	asked["summary"] = "Is the error Decode returns meant to be dropped?"
	asked["anchor"].(map[string]any)["start_line"] = recordUnitStart[gapUnit] + 5
	asked["anchor"].(map[string]any)["line"] = recordUnitStart[gapUnit] + 5

	_, err := runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson", asserted, asked), "--repo", recordSlug)
	require.NoError(t, err)
	stored, err := state.ReadStamped[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings, recordRound)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	for i := range stored {
		require.Equal(t, finding.GradeArgued, stored[i].Grade, "%s rests on nothing cr established", stored[i].ID)
		require.Equal(t, finding.KindQuestion, stored[i].Kind, "and is stored as a question either way")
	}

	one := finding.Forcings{{Class: "unchecked-error", Count: 1}}
	for _, run := range []string{"cr draft", "cr draft regenerated"} {
		printed, err := runDraft(t, recordPR, "--repo", recordSlug)
		require.NoError(t, err)
		assert.Equal(t, one, forcedOf(t, []byte(printed)),
			"%s: §6.3.2 counts f1, which §6.3.1 changed, and not f2, which its role asked", run)
		assert.Equal(t, one, forcedOf(t, recordSummary(t, layout)),
			"%s: the round summary carries the same count", run)
	}

	printed, err := runPost(t, recordPR, "--repo", recordSlug)
	require.NoError(t, err)
	assert.Equal(t, one, forcedOf(t, []byte(printed)), "cr post's dry run reports the same count")
}
