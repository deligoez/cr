package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// ledger names every §7.3.1 event the repository holds as
// "<record>:<action>", in the order triage.ndjson holds them.
func ledger(t *testing.T, layout state.Layout) []string {
	t.Helper()
	events, err := finding.TriageEvents(layout, draftOwner, draftRepo)
	require.NoError(t, err)
	named := make([]string, 0, len(events))
	for i := range events {
		named = append(named, events[i].Record+":"+string(events[i].Action))
	}
	return named
}

// §7.3.1 through the command: `cr draft` writes one `raised` event per record
// it queues and the outcome event for the records it discards at draft time,
// and nothing else.
//
// The softened record is the assertion that matters beside the two discards.
// §7.3.1 gives `cr post --confirm` the outcome per queued record, and a
// softening leaves the record queued — it is still open to every verb the next
// reading of the draft admits — so an outcome written for it here would be an
// answer to a question the round has not finished asking.
//
// The third run is where an event would be appended rather than overwritten if
// it were going to be: three regenerations of five records leave seven events,
// not seventeen.
func TestADraftRaisesWhatItQueuedAndSettlesOnlyItsDiscards(t *testing.T) {
	records := make([]*finding.Finding, 0, 5)
	for _, id := range []string{"f1", "f2", "f3", "f4", "f5"} {
		record := aStoredRecord(id, finding.StateDraft)
		record.Anchor.Path = "internal/api/" + id + ".go"
		records = append(records, record)
	}
	layout := draftedHome(t, records...)

	redraft(t)
	assert.Equal(t,
		[]string{"f1:raised", "f2:raised", "f3:raised", "f4:raised", "f5:raised"},
		ledger(t, layout), "§7.3.1: one raised event per record the draft queued")

	edited := markerEdit(t, readDraft(t, layout), "f3", `kind="finding"`, `kind="question"`)
	edited = deleteBlock(t, edited, "f4")
	edited = markerEdit(t, edited, "f5", `disposition=""`, `disposition="wrong"`)
	writeDraft(t, layout, edited)

	redraft(t)
	settled := []string{
		"f1:raised", "f2:raised", "f3:raised", "f4:raised", "f5:raised",
		"f4:discarded-not-here", "f5:discarded-wrong",
	}
	assert.Equal(t, settled, ledger(t, layout),
		"the two discards settle here, the softening does not, and every raise survives")

	redraft(t)
	assert.Equal(t, settled, ledger(t, layout),
		"§7.3.1: a regeneration overwrites each event under its key rather than appending")
}

// The event carries the record's §7.3.1 fields and the occasion's, so §7.3.2
// can report per class and per rule without reading a findings.ndjson that
// belongs to one pull request out of the repository's many.
func TestADiscardsEventCarriesItsClassRuleAndOccasion(t *testing.T) {
	record := aStoredRecord("f1", finding.StateDraft)
	record.Rule = "no-dropped-error"
	layout := draftedHome(t, record)

	redraft(t)
	writeDraft(t, layout,
		markerEdit(t, readDraft(t, layout), "f1", `disposition=""`, `disposition="wrong"`))
	redraft(t)

	events, err := finding.TriageEvents(layout, draftOwner, draftRepo)
	require.NoError(t, err)
	require.Len(t, events, 2)
	outcome := events[1]
	assert.Equal(t, finding.TriageAction(finding.OutcomeDiscardedWrong), outcome.Action)
	assert.Equal(t, "unchecked-error", outcome.Class)
	assert.Equal(t, "no-dropped-error", outcome.Rule)
	assert.Equal(t, "correctness", outcome.Axis)
	assert.Equal(t, "correctness", outcome.Role)
	assert.Equal(t, finding.GradeCited, outcome.Grade)
	assert.Equal(t, draftPRNum, outcome.PR)
	assert.Equal(t, draftRound, outcome.Round)
	assert.Equal(t, draftHead, outcome.Head)
	assert.False(t, outcome.At.IsZero(), "§7.3.3's first-seen ordering is the timestamp's")
}

// §7.3.1: `cr post` without `--confirm` writes no triage event, since it
// changes nothing.
//
// The run is driven over a draft the reviewer discarded a record in, which is
// the case a writer would most plausibly be tempted into: the command reads the
// discard and leaves it out of the payload, and reading a decision is not
// acting on one.
func TestAPostWithoutConfirmWritesNoTriageEvent(t *testing.T) {
	records := []*finding.Finding{aCitedRecord("f1"), aCitedRecord("f2")}
	records[1].Anchor.Path = "internal/api/f2.go"
	layout := draftedHome(t, records...)
	redraft(t)
	writeDraft(t, layout,
		markerEdit(t, readDraft(t, layout), "f2", `disposition=""`, `disposition="wrong"`))

	before := ledger(t, layout)
	printed, err := runPost(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)

	assert.Equal(t, before, ledger(t, layout),
		"§7.3.1: a run that changes nothing counts nothing")
	assert.Equal(t, []string{"f1:raised", "f2:raised"}, before)
	report := postReportOf(t, printed)
	assert.Equal(t, []postedComment{{ID: "f1", Kind: finding.KindFinding}}, report.Comments,
		"and the discarded record is left out of the payload")
	assert.Equal(t, []string{"f2"}, report.Discarded, "and named as the discard the run read")
}

// The seam `cr post --confirm` writes its outcomes through, driven over the
// four actions §7.3.1 draws them from.
//
// It is exercised here rather than through the command because the network
// write itself is the confirmation gate's, and round 8's
// triage-event-key-permits-contradiction is about what is written once that
// call has returned: one outcome per queued record, whichever verb the reviewer
// used, including the two the draft already settled — so a record's ledger
// entry after a post is the post's answer and not the draft's.
func TestThePostSeamSettlesEveryQueuedRecordOnce(t *testing.T) {
	records := make([]*finding.Finding, 0, 4)
	for _, id := range []string{"f1", "f2", "f3", "f4"} {
		record := aStoredRecord(id, finding.StateDraft)
		record.Anchor.Path = "internal/api/" + id + ".go"
		records = append(records, record)
	}
	layout := draftedHome(t, records...)
	redraft(t)
	edited := markerEdit(t, readDraft(t, layout), "f2", `kind="finding"`, `kind="question"`)
	edited = deleteBlock(t, edited, "f3")
	edited = markerEdit(t, edited, "f4", `disposition=""`, `disposition="wrong"`)
	writeDraft(t, layout, edited)

	round := state.Meta{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum,
		Round: draftRound, Head: draftHead,
	}
	stored, err := roundFindingsOf(layout, draftOwner, draftRepo, draftPRNum, draftRound)
	require.NoError(t, err)
	triage, err := ingestDraft(layout, draftOwner, draftRepo, draftPRNum, &round, stored,
		finding.NewJournal(finding.ActorPostConfirm, draftHead, time.Now()))
	require.NoError(t, err)
	require.NoError(t, recordPostTriage(
		layout, draftOwner, draftRepo, draftPRNum, &round, triage.settled()))

	assert.Equal(t, []string{
		"f1:raised", "f2:raised", "f3:raised", "f4:raised",
		"f1:kept", "f2:softened", "f3:discarded-not-here", "f4:discarded-wrong",
	}, ledger(t, layout), "§7.3.1: exactly one outcome per queued record, drawn from the four")
}
