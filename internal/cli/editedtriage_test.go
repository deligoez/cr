package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// editedMarks are the `edited` marks of the repository's outcome events, by
// record id, "absent" where an event carries none.
func editedMarks(t *testing.T, layout state.Layout) map[string]string {
	t.Helper()
	events, err := finding.TriageEvents(layout, draftOwner, draftRepo)
	require.NoError(t, err)
	marks := make(map[string]string)
	for i := range events {
		if events[i].Action == finding.ActionRaised {
			continue
		}
		mark := "absent"
		if events[i].Edited != nil {
			mark = map[bool]string{true: "true", false: "false"}[*events[i].Edited]
		}
		marks[events[i].Record+":"+string(events[i].Action)] = mark
	}
	return marks
}

// §7.3.2 through `cr post --confirm`: a `kept` event carries `edited: true`
// when the body posted differs from the record's rendered.json entry, and
// `edited: false` when the human sent the body `cr draft` rendered.
func TestAConfirmedPostMarksWhetherEachKeptBodyWasEdited(t *testing.T) {
	first := aCitedRecord("f1")
	layout := draftedHome(t, first, aCitedRecord("f2"))
	redraft(t)
	rewritten := strings.Replace(readDraft(t, layout), first.Summary+"\n\n"+first.Evidence,
		"The reviewer's own sentence: Decode's error is dropped on this line.", 1)
	require.NotEqual(t, readDraft(t, layout), rewritten, "the edit reached f1's body")
	require.NoError(t, os.WriteFile(
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft),
		[]byte(rewritten), 0o600))
	ghShimming(t, builtPayload(t))

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"f1:kept": "true", "f2:kept": "false"}, editedMarks(t, layout))
}

// §7.3.2 through `cr stats`: per class and per rule, how many of the kept and
// softened were posted unedited. An event carrying no mark, written before cr
// recorded one, is counted apart and never as unedited.
func TestStatsCountsTheKeptAndSoftenedPostedUnedited(t *testing.T) {
	layout := statsHome(t)
	edited, unedited := true, false
	records := []*finding.Finding{
		aTriagedRecord("f1", "unchecked-error", "no-dropped-error"),
		aTriagedRecord("f2", "unchecked-error", "no-dropped-error"),
		aTriagedRecord("f3", "unchecked-error", ""),
		aTriagedRecord("f4", "unchecked-error", ""),
		aTriagedRecord("f5", "unchecked-error", ""),
	}
	seedRaised(t, layout, statsFirst, 1, records...)
	require.NoError(t, finding.RecordOutcomes(layout, statsOwner, statsRepo, []finding.Settled{
		{Record: records[0], Outcome: finding.OutcomeKept, Edited: &unedited},
		{Record: records[1], Outcome: finding.OutcomeSoftened, Edited: &edited},
		{Record: records[2], Outcome: finding.OutcomeSoftened, Edited: &unedited},
		{Record: records[3], Outcome: finding.OutcomeKept},
		{Record: records[4], Outcome: finding.OutcomeDiscardedWrong, Edited: &unedited},
	}, occasionOf(statsFirst, 1)))

	report := statsReport(t)

	assert.Equal(t, []finding.ClassTriage{{Class: "unchecked-error", TriageCounts: finding.TriageCounts{
		Raised: 5, Kept: 2, Softened: 2, DiscardedWrong: 1, Unedited: 2, EditUnknown: 1,
	}}}, report.Classes, "a discard is neither kept nor softened, so its mark is not counted")
	assert.Equal(t, []finding.RuleTriage{{Rule: "no-dropped-error", TriageCounts: finding.TriageCounts{
		Raised: 2, Kept: 1, Softened: 1, Unedited: 1,
	}}}, report.Rules)
}
