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

// regenerated is what a `cr draft` run reported about the draft it replaced.
type regenerated struct {
	Triaged   []triagedRecord `json:"triaged"`
	Preserved []string        `json:"preserved"`
}

// redraft runs `cr draft` and decodes what it reported.
func redraft(t *testing.T) regenerated {
	t.Helper()
	printed, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)
	var got regenerated
	require.NoError(t, json.Unmarshal([]byte(printed), &got))
	return got
}

// deleteBlock removes one record's block from a draft, marker and body, the way
// a reviewer deletes it.
func deleteBlock(t *testing.T, file, id string) string {
	t.Helper()
	start := strings.Index(file, `<!-- cr:record id="`+id+`"`)
	require.GreaterOrEqual(t, start, 0)
	end := strings.Index(file[start+1:], "<!-- cr:record ")
	if end < 0 {
		return file[:start]
	}
	return file[:start] + file[start+1+end:]
}

// §7.1.6 through the command, over the four cases the criterion names: an
// untouched block, a whitespace-only edit, a substantive edit, and a deletion
// followed by regeneration — and then a third run, which is where a deleted
// block would come back and a preserved body would be overwritten if either
// were going to be.
//
// The whitespace-only edit is two trailing spaces, a Markdown line break. The
// comparison is byte-exact, so it is kept exactly as typed.
func TestRegenerationKeepsEditsDiscardsDeletionsAndResurrectsNothing(t *testing.T) {
	records := []*finding.Finding{
		aStoredRecord("f1", finding.StateDraft), aStoredRecord("f2", finding.StateDraft),
		aStoredRecord("f3", finding.StateDraft), aStoredRecord("f4", finding.StateDraft),
	}
	records[3].Anchor.Path = "internal/api/money.go"
	layout := draftedHome(t, records...)
	generated := func(r *finding.Finding) string { return r.Summary + "\n\n" + r.Evidence }
	redraft(t)

	whitespace := strings.Replace(generated(records[1]), "f2.", "f2.  ", 1)
	substantive := "The reviewer's own wording: nothing downstream ever sees Decode's error."
	edited := strings.Replace(readDraft(t, layout), generated(records[1]), whitespace, 1)
	edited = strings.Replace(edited, generated(records[2]), substantive, 1)
	edited = deleteBlock(t, edited, "f4")
	draftFile := layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft)
	require.NoError(t, os.WriteFile(draftFile, []byte(edited), 0o600))

	second := redraft(t)
	assert.Equal(t, regenerated{
		Triaged:   []triagedRecord{{ID: "f4", Outcome: finding.OutcomeDiscardedNotHere}},
		Preserved: []string{"f2", "f3"},
	}, second)
	assertRegenerated(t, layout, generated(records[0]), whitespace, substantive)

	third := redraft(t)
	assert.Equal(t, regenerated{Triaged: []triagedRecord{}, Preserved: []string{"f2", "f3"}}, third,
		"the third run discards nothing new and still keeps both edits")
	assertRegenerated(t, layout, generated(records[0]), whitespace, substantive)
}

// assertRegenerated checks the round after a regeneration: the draft's blocks
// and bodies, f4's discard and its one waiver, and rendered.json's entries.
func assertRegenerated(t *testing.T, layout state.Layout, untouched, whitespace, substantive string) {
	t.Helper()
	file := readDraft(t, layout)
	assert.Equal(t, []string{"f1", "f2", "f3"}, markersIn(file), "§7.1.6: the deleted block is not resurrected")
	assert.Contains(t, file, untouched, "the untouched block is rendered as before")
	assert.Contains(t, file, "\n"+whitespace+"\n", "the whitespace edit is kept byte for byte")
	assert.Contains(t, file, "\n"+substantive+"\n", "the substantive edit is kept")

	stored, err := state.ReadRecords[finding.Finding](layout, draftOwner, draftRepo, draftPRNum, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 4)
	assert.Equal(t, finding.StateDiscarded, stored[3].State, "§9.1: cr draft moves a deleted block's record to discarded")
	assert.Equal(t, finding.DispositionNotHere, stored[3].Disposition, "§7.2: a deletion is not-here")
	for i := range stored[:3] {
		assert.Equal(t, finding.StateQueued, stored[i].State, stored[i].ID)
	}

	waivers, err := finding.PullRequestWaivers(layout, draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.Len(t, waivers, 1, "§7.2: one pull-request-scoped waiver, written once however often the draft regenerates")
	assert.Equal(t, "internal/api/money.go", waivers[0].Path)
	assert.Equal(t, finding.DispositionNotHere, waivers[0].Disposition)

	entries := readRendered(t, layout)
	assert.Len(t, entries, 3, "rendered.json holds the records the draft holds")
	for id, record := range map[string]string{"f2": whitespace, "f3": substantive} {
		var entry string
		require.NoError(t, json.Unmarshal(entries[id], &entry))
		assert.NotEqual(t, record, entry, "§7.1.5: a preserved body does not replace %s's entry", id)
	}
}
