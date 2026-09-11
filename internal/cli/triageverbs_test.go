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

// markerEdit changes one field of one record's marker line in a draft, the way
// a reviewer types into it.
func markerEdit(t *testing.T, file, id, from, to string) string {
	t.Helper()
	lines := strings.Split(file, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, `<!-- cr:record id="`+id+`"`) {
			require.Contains(t, line, from)
			lines[i] = strings.Replace(line, from, to, 1)
			return strings.Join(lines, "\n")
		}
	}
	require.Failf(t, "no marker", "the draft holds no block for %s", id)
	return file
}

// waiverScopeOf names the file of §7.4.4 holding a waiver for path, or ""
// when neither does.
func waiverScopeOf(t *testing.T, layout state.Layout, path string) string {
	t.Helper()
	here, err := finding.PullRequestWaivers(layout, draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	wide, err := finding.RepositoryWaivers(layout, draftOwner, draftRepo)
	require.NoError(t, err)
	scope := ""
	for _, file := range []struct {
		name    string
		waivers []finding.WaiverRecord
	}{{"pull request", here}, {"repository", wide}} {
		for i := range file.waivers {
			if file.waivers[i].Path == path {
				require.Empty(t, scope, "a waiver for %s sits in both files", path)
				scope = file.name
			}
		}
	}
	return scope
}

// §7.2's five verbs through the command, each on its own record, and for each
// the disposition it leaves, the scope of the waiver it writes, and its
// contribution to §7.3's statistics.
//
// f5 is marked wrong with its body untouched, which is the case §7.2 names:
// the reviewer never has to delete text to say a finding is false. Every record
// is anchored in a file of its own so each waiver is attributable to exactly
// one verb.
func TestTheFiveTriageVerbsOfSection72(t *testing.T) {
	records := make([]*finding.Finding, 0, 5)
	for _, id := range []string{"f1", "f2", "f3", "f4", "f5"} {
		record := aStoredRecord(id, finding.StateDraft)
		record.Anchor.Path = "internal/api/" + id + ".go"
		records = append(records, record)
	}
	layout := draftedHome(t, records...)
	redraft(t)

	edited := strings.Replace(readDraft(t, layout), records[1].Summary, "The reviewer's own wording of f2.", 1)
	edited = markerEdit(t, edited, "f3", `kind="finding"`, `kind="question"`)
	edited = deleteBlock(t, edited, "f4")
	edited = markerEdit(t, edited, "f5", `disposition=""`, `disposition="wrong"`)
	require.NoError(t, os.WriteFile(
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft), []byte(edited), 0o600))

	report := redraft(t)

	stored, err := state.ReadRecords[finding.Finding](layout, draftOwner, draftRepo, draftPRNum, state.FileFindings)
	require.NoError(t, err)
	outcomes := map[string]finding.Outcome{}
	for _, triaged := range report.Triaged {
		outcomes[triaged.ID] = triaged.Outcome
		assert.Equal(t, triaged.Outcome.CountsAgainstClass(), triaged.CountsAgainstClass, triaged.ID)
	}
	for i, verb := range []struct {
		name        string
		disposition finding.Disposition
		waiver      string
		outcome     finding.Outcome
		counts      bool
	}{
		{"left unchanged", "", "", "", false},
		{"body edited", "", "", "", false},
		{"kind softened", "", "", finding.OutcomeSoftened, true},
		{"block deleted", finding.DispositionNotHere, "pull request", finding.OutcomeDiscardedNotHere, false},
		{"marked wrong", finding.DispositionWrong, "repository", finding.OutcomeDiscardedWrong, true},
	} {
		id := records[i].ID
		assert.Equal(t, verb.disposition, stored[i].Disposition, "%s: disposition", verb.name)
		assert.Equal(t, verb.waiver, waiverScopeOf(t, layout, records[i].Anchor.Path), "%s: waiver scope", verb.name)
		assert.Equal(t, verb.outcome, outcomes[id], "%s: outcome, where kept is not reported", verb.name)
		assert.Equal(t, verb.counts, outcomes[id].CountsAgainstClass(), "%s: counted against the class", verb.name)
	}

	file := readDraft(t, layout)
	assert.Equal(t, []string{"f1", "f2", "f3"}, markersIn(file), "both discards leave the draft")
	assert.Contains(t, file, `id="f3" kind="question"`, "the softened block reads as a question")
	assert.Equal(t, finding.KindFinding, stored[2].Kind,
		"and stays a finding in state, so the next reading of the draft softens it again")
	assert.Equal(t, []string{"f2"}, report.Preserved, "the edited body is posted as edited")
	assert.Contains(t, file, "The reviewer's own wording of f2.")
}

// §12.1's other shape for a regeneration. A terminal reader is told, record by
// record, what cr acted on — each outcome, and whether it counts against the
// class — and whose edited body was kept, because a discard wrote a waiver on
// their behalf and they are owed the list rather than a count.
func TestATerminalRegenerationNamesWhatItTriaged(t *testing.T) {
	records := []*finding.Finding{
		aStoredRecord("f1", finding.StateDraft), aStoredRecord("f2", finding.StateDraft),
		aStoredRecord("f3", finding.StateDraft),
	}
	records[1].Anchor.Path = "internal/api/f2.go"
	records[2].Anchor.Path = "internal/api/f3.go"
	layout := draftedHome(t, records...)
	redraft(t)
	edited := strings.Replace(readDraft(t, layout), records[0].Summary, "The reviewer's own wording of f1.", 1)
	edited = deleteBlock(t, edited, "f2")
	edited = markerEdit(t, edited, "f3", `disposition=""`, `disposition="wrong"`)
	require.NoError(t, os.WriteFile(
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft), []byte(edited), 0o600))

	out := throughATerminal(t, "draft", draftPR, "--repo", draftSlug)

	assert.Contains(t, out, "\nf2: discarded-not-here\n", "a deletion is named and not counted against the class")
	assert.Contains(t, out, "\nf3: discarded-wrong, counted against its class\n")
	assert.Contains(t, out, "\nkept the edited body of: f1\n")
}
