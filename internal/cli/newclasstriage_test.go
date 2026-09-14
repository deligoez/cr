package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/finding"
)

// §7.3.3 through the command, after a triage has discarded every record of both
// of the round's new classes: the regenerated draft still reports both, in its
// output and in summary.json, as the first rendering did.
//
// QA D-S10-1: one block deleted and the other marked `wrong`, and the next
// `cr draft` wrote `new_classes: []` over `[naming, reinvention]` while
// `cr stats` still named both as first seen on the round. The report is about
// the round, not about the draft that happens to be current, so a discard is
// not the run that makes a class old.
//
// f3 is a duplicate of a class of its own. No draft ever raised it, so its class
// is never reported, before the triage or after: that is what keeps the report
// from being every class the round's file holds.
func TestANewClassSurvivesTheDiscardOfEveryRecordOfIt(t *testing.T) {
	reinvention, naming := aStoredRecord("f1", finding.StateDraft), aStoredRecord("f2", finding.StateDraft)
	reinvention.Class = "reinvention"
	naming.Class = "naming"
	naming.Anchor.Path = "internal/api/money.go"
	duplicate := aStoredRecord("f3", finding.StateDuplicate)
	duplicate.Class = "never-raised"
	layout := draftedHome(t, reinvention, naming, duplicate)

	first := draftedNewClasses(t)
	assert.Equal(t, []string{"naming", "reinvention"}, first,
		"the repository has no history, so both raised classes are new and the duplicate's is not")
	assert.Equal(t, first, summaryNewClassesOf(t, layout))

	edited := deleteBlock(t, readDraft(t, layout), "f2")
	writeDraft(t, layout, markerEdit(t, edited, "f1", `disposition=""`, `disposition="wrong"`))

	assert.Equal(t, first, draftedNewClasses(t),
		"§7.3.3: the draft that discards both records still reports the round's new classes")
	assert.Equal(t, first, summaryNewClassesOf(t, layout),
		"§7.3.3: and summary.json is not overwritten with a report that forgot them")
	for _, record := range draftedFindings(t, layout)[:2] {
		assert.Equal(t, finding.StateDiscarded, record.State, "the triage discarded %s", record.ID)
	}

	assert.Equal(t, first, draftedNewClasses(t),
		"a draft with no block left regenerates the same report")
	assert.Equal(t, first, summaryNewClassesOf(t, layout))
}

// The same report after the round was posted: a record `cr post --confirm`
// moved to `posted` was raised by the round's draft, so a draft run afterwards
// still names its class.
func TestANewClassSurvivesThePostingOfItsRecords(t *testing.T) {
	record := aCitedRecord("f1")
	record.Class = "reinvention"
	layout := draftedHome(t, record)
	first := draftedNewClasses(t)
	assert.Equal(t, []string{"reinvention"}, first)
	ghShimming(t, builtPayload(t))
	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	assert.NoError(t, err)
	assert.Equal(t, finding.StatePosted, draftedFindings(t, layout)[0].State)

	assert.Equal(t, first, draftedNewClasses(t),
		"§7.3.3: a draft after the send still reports the round's new class")
	assert.Equal(t, first, summaryNewClassesOf(t, layout))
}
