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

