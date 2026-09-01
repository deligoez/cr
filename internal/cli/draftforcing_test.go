package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// anArguedRecord is a record §6.2 graded `argued`, stored in the register the
// agent wrote rather than the one §6.3 forces it into.
//
// Storing it as an assertion is what makes the draft-time moment provable.
// §6.3.1 forces at record time too, so a fixture already carrying `question`
// would leave this command nothing to do and the test would pass whether or not
// §6.3.1's second moment ran at all.
func anArguedRecord(id, class string) *finding.Finding {
	record := aStoredRecord(id, finding.StateDraft)
	record.Class = class
	record.Grade = finding.GradeArgued
	record.Kind = finding.KindFinding
	return record
}

// readSummary reads the round's summary.json off disk as its raw fields.
func readSummary(t *testing.T, layout state.Layout) map[string]json.RawMessage {
	t.Helper()
	body, err := os.ReadFile(
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileSummary))
	require.NoError(t, err)
	var summary map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &summary))
	return summary
}

// §6.3.1's second moment through the command, and §6.3.2 beside it: the argued
// records reach the draft as questions, and the count per class reaches the
// round summary.
//
// Two classes with different counts, so the report is a breakdown rather than a
// total wearing a class name. The cited record is there for the same reason
// `cr record`'s forcing test keeps one: it proves the command rewrote the
// register §6.2 left with nothing behind it, and not every register it was
// given.
func TestDraftForcesArguedRecordsAndCountsThemPerClass(t *testing.T) {
	layout := draftedHome(t,
		anArguedRecord("f1", "unchecked-error"),
		aStoredRecord("f2", finding.StateDraft),
		anArguedRecord("f3", "naming-drift"),
		anArguedRecord("f4", "unchecked-error"),
	)

	printed, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)

	rendered := readDraft(t, layout)
	assert.Contains(t, rendered, `id="f1" kind="question"`,
		"§6.3.1: the forcing is applied again at draft time")
	assert.Contains(t, rendered, `id="f3" kind="question"`)
	assert.Contains(t, rendered, `id="f2" kind="finding"`,
		"§6.2 lets a cited record assert, so the agent's register stands")

	stored, err := state.ReadRecords[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings)
	require.NoError(t, err)
	assert.Equal(t, finding.KindQuestion, stored[0].Kind,
		"and the forcing reaches the file, not only the rendering")

	var payload struct {
		Forced finding.Forcings `json:"forced_to_question"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &payload))
	assert.Equal(t, finding.Forcings{
		{Class: "naming-drift", Count: 1},
		{Class: "unchecked-error", Count: 2},
	}, payload.Forced, "§6.3.2: a count per class, by class ascending")

	var summarised finding.Forcings
	require.NoError(t, json.Unmarshal(readSummary(t, layout)["forced_to_question"], &summarised))
	assert.Equal(t, payload.Forced, summarised,
		"§6.3.2 and §10.3: the round summary carries the same count the run reported")
}

// §11.1 exempts §6.3.2's forcing counts from `--quiet`, in either output shape.
//
// The document and the terminal are both checked because the flag could take
// the disclosure out of either one on its own: a field dropped from the JSON
// leaves the agent parsing a round that forced nothing, and a line dropped from
// the text leaves the person at the terminal reading the same.
func TestTheForcingCountSurvivesQuiet(t *testing.T) {
	for _, flags := range [][]string{
		{},
		{"--quiet"},
		{"--quiet", "--compact"},
		{"--quiet", "--no-color"},
	} {
		t.Run("cr draft "+flagsNamed(flags), func(t *testing.T) {
			draftedHome(t, anArguedRecord("f1", "unchecked-error"))

			printed, err := runDraft(t, append(
				[]string{draftPR, "--repo", draftSlug}, flags...)...)
			require.NoError(t, err)
			assert.Contains(t, printed, `"forced_to_question"`,
				"§11.1: --quiet may not take an honesty disclosure out of the document")

			terminal := throughATerminal(t,
				append([]string{"draft", draftPR, "--repo", draftSlug}, flags...)...)
			assert.Contains(t, terminal, "§6.3: 1 records forced to question",
				"§11.1: nor out of the terminal")
			assert.Contains(t, terminal, "unchecked-error 1",
				"and the count is per class there too")
		})
	}
}

// The disclosure is printed on a round that forced nothing, so a reader can
// tell a round with no argued records from a round where the forcing never ran.
func TestTheForcingCountIsPrintedAtZero(t *testing.T) {
	draftedHome(t, aStoredRecord("f1", finding.StateDraft))

	printed, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)
	assert.Contains(t, printed, `"forced_to_question": []`,
		"§12.3: an empty count is an empty array, never null")

	terminal := throughATerminal(t, "draft", draftPR, "--repo", draftSlug)
	assert.Contains(t, terminal, "§6.3: 0 records forced to question")
}

// §10.3 has three commands accumulate their counts into one summary.json, so
// `cr draft` writing its own section must leave every other section alone.
//
// The failure this closes is silent and total: a writer that decoded the
// document into the one section it owns would re-encode it without the rest,
// and the round's history — which §10.3 exists to make reconstructable from
// state alone — would lose whatever `cr merge` had put there, every time a
// draft was regenerated.
func TestDraftLeavesTheOtherSummaryCountsAlone(t *testing.T) {
	layout := draftedHome(t, anArguedRecord("f1", "unchecked-error"))

	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteRound(draftRound, state.FileSummary,
		[]byte(`{"waived": 3, "deduplicated": 1}`+"\n")))
	require.NoError(t, held.Unlock())

	_, err = runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)

	summary := readSummary(t, layout)
	assert.Equal(t, "3", string(summary["waived"]), "another writer's count survives")
	assert.Equal(t, "1", string(summary["deduplicated"]))
	assert.Contains(t, string(summary["forced_to_question"]), "unchecked-error")
}

// A second `cr draft` reports the same count as the first.
//
// This is the property that decides whether §6.3.2's report says anything.
// §6.3.1 applies the forcing three times, so by the second run every record it
// moved is already a question and a report counting the moves this run made
// would read zero — at exactly the moment a reviewer regenerating their draft
// is looking at it.
func TestTheForcingCountDoesNotFadeOnASecondRun(t *testing.T) {
	draftedHome(t, anArguedRecord("f1", "unchecked-error"))

	first, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)
	second, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)

	forced := func(printed string) string {
		var payload map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(printed), &payload))
		return string(payload["forced_to_question"])
	}
	assert.Equal(t, forced(first), forced(second),
		"§6.3.2 counts the records the forcing holds, not the moves one run made")
	assert.Contains(t, forced(second), "unchecked-error")
	assert.True(t, strings.Contains(forced(second), `"count":1`) ||
		strings.Contains(forced(second), `"count": 1`))
}
