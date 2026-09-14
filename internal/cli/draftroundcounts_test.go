package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// draftedCounts is what a `cr draft` run reported about the round's counts.
type draftedCounts struct {
	Triaged  []triagedRecord  `json:"triaged"`
	Forced   finding.Forcings `json:"forced_to_question"`
	Comments *summaryCap      `json:"comments"`
}

// draftCounts runs `cr draft` with flags and decodes the counts it reported.
func draftCounts(t *testing.T, flags ...string) draftedCounts {
	t.Helper()
	printed, err := runDraft(t, append([]string{draftPR, "--repo", draftSlug}, flags...)...)
	require.NoError(t, err)
	var got draftedCounts
	require.NoError(t, json.Unmarshal([]byte(printed), &got))
	return got
}

// summarisedForcing is the forced_to_question summary.json holds.
func summarisedForcing(t *testing.T, layout state.Layout) finding.Forcings {
	t.Helper()
	var summarised finding.Forcings
	require.NoError(t, json.Unmarshal(readSummary(t, layout)["forced_to_question"], &summarised))
	return summarised
}

// §11.1 through the command: `cr draft` prints §1.6.2's comment count against
// post.max_comments in its document and on a terminal, `--quiet` or not.
//
// QA D-S08-2: at three comments against a cap of one, the count was in
// draft.md's header and summary.json and in neither output, so the agent
// reading `cr draft` learned the round was over the cap only when `cr post`
// refused it.
func TestADraftPrintsItsCommentCountAgainstTheCap(t *testing.T) {
	for _, flags := range [][]string{{}, {"--quiet"}} {
		t.Run("cr draft "+flagsNamed(flags), func(t *testing.T) {
			layout := draftedHome(t,
				aStoredRecord("f1", finding.StateDraft),
				aStoredRecord("f2", finding.StateDraft),
				aStoredRecord("f3", finding.StateDraft),
			)
			require.NoError(t, layout.EnsureRepo(draftOwner, draftRepo))
			require.NoError(t, os.WriteFile(layout.RepoConfig(draftOwner, draftRepo),
				[]byte(`{"post": {"max_comments": 1}}`), 0o600))

			assert.Equal(t, &summaryCap{Count: 3, Max: 1}, draftCounts(t, flags...).Comments,
				"§11.1: the document carries the count and the cap it was measured against")

			terminal := throughATerminal(t, append([]string{"draft", draftPR, "--repo", draftSlug}, flags...)...)
			assert.Contains(t, terminal, "\n§1.6.2: 3 comments queued against post.max_comments 1, 2 over the cap\n",
				"§11.1: and so does the terminal")
		})
	}
}

// §6.3.2 through the command, after a triage has deleted a record the round's
// forcing moved: the regenerated draft still counts it, in its output and in
// summary.json.
//
// QA D-S08-1: deleting the one forced record's block made the next draft write
// `forced_to_question: []` beside `forced_records` still naming it.
//
// f3 is a second forced record of the same class the reviewer keeps, so the
// count is two before and after — a record counted once as queued and again as
// settled would read three. f2 is cited and deleted too, and was never forced,
// and f4 is a discarded record named among the forced ids whose grade is not
// `argued`: neither is counted.
func TestADraftKeepsTheForcingCountOfADiscardedRecord(t *testing.T) {
	cited := aStoredRecord("f2", finding.StateDraft)
	cited.Anchor.Path = "internal/api/money.go"
	regraded := aStoredRecord("f4", finding.StateDraft)
	regraded.Class = "regraded"
	regraded.Anchor.Path = "internal/api/regraded.go"
	deleted := anArguedRecord("f1", "untested-branch")
	deleted.Anchor.Path = "internal/api/branch.go"
	layout := draftedHome(t, deleted, cited, anArguedRecord("f3", "untested-branch"), regraded)
	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.NoError(t, keepForced(held, layout, &state.Meta{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum, Round: draftRound, Head: draftHead,
	}, []string{"f4"}))
	require.NoError(t, held.Unlock())

	want := finding.Forcings{{Class: "untested-branch", Count: 2}}
	assert.Equal(t, want, draftCounts(t).Forced)

	edited := deleteBlock(t, readDraft(t, layout), "f1")
	edited = deleteBlock(t, edited, "f2")
	writeDraft(t, layout, deleteBlock(t, edited, "f4"))

	after := draftCounts(t)
	require.Len(t, after.Triaged, 3, "the triage discarded f1, f2 and f4")
	assert.Equal(t, want, after.Forced,
		"§6.3.2: the forcing happened in this round, and deleting the block does not undo it")
	assert.Equal(t, want, summarisedForcing(t, layout),
		"§10.3: summary.json is not overwritten with a count that forgot it")
	assert.Equal(t, want, draftCounts(t).Forced, "a further regeneration says the same")
}

// §6.3.1's draft-time forcing of a stored argued finding is reported as a
// forcing, not as the reviewer's softening.
//
// QA D-S08-3: findings.ndjson's argued question was edited to a finding by
// hand while draft.md's marker still read question, and `cr draft` reported
// `softened`, counted against the class per §7.3.4, with `forced_to_question`
// silent. The marker says what cr rendered, so nothing was triaged.
func TestADraftForcingAStoredArguedFindingIsNotASoftening(t *testing.T) {
	asked := anArguedRecord("f1", "tax-rounding")
	asked.Kind = finding.KindQuestion
	layout := draftedHome(t, asked)
	first := draftCounts(t)
	require.Empty(t, first.Forced, "the role wrote a question, so nothing was forced")

	edited := aStoredRecord("f1", finding.StateQueued)
	edited.Class, edited.Grade = "tax-rounding", finding.GradeArgued
	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.NoError(t, state.ReplaceStamped(held, state.FileFindings,
		state.Stamp{Head: draftHead, Round: draftRound}, []*finding.Finding{edited}))
	require.NoError(t, held.Unlock())

	got := draftCounts(t)
	assert.Equal(t, []triagedRecord{}, got.Triaged,
		"§7.2: the marker still reads what cr rendered, so the reviewer softened nothing")
	assert.Equal(t, finding.Forcings{{Class: "tax-rounding", Count: 1}}, got.Forced,
		"§6.3.2: the draft's forcing moved the record, and the report counts it")
	assert.Equal(t, finding.KindQuestion, draftedFindings(t, layout)[0].Kind)
}
