package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// severitiesIn names each event as "<record>:<action>:<severity>", in the order
// triage.ndjson holds them, so a test can say where a severity change became
// observable without depending on anything else the event carries.
func severitiesIn(t *testing.T, layout state.Layout) []string {
	t.Helper()
	events, err := finding.TriageEvents(layout, draftOwner, draftRepo)
	require.NoError(t, err)
	named := make([]string, 0, len(events))
	for i := range events {
		named = append(named,
			events[i].Record+":"+string(events[i].Action)+":"+string(events[i].Severity))
	}
	return named
}

// Round 8's unnameable-triage-event, driven end to end: a record whose only
// draft edit is a severity change is recorded on its single outcome event, and
// the ledger gains no event of its own for it.
//
// The finding's two options were a sixth action name or the field, and this is
// the field. What makes it checkable is the pair of events §7.3.1 already
// required: the raise carries the severity the record was raised at, the
// outcome carries the one the reviewer settled on, and the change is the
// difference between them. So the assertion is not merely that the new value is
// somewhere — it is that it appears on exactly one of the two, that the ledger
// holds exactly those two, and that every action in it is still one of the five
// §7.3.1 calls complete.
//
// The reviewer edits the marker and does not regenerate the draft, which is the
// ordinary path: §8.5.1's dry run and then `cr post --confirm` is what a
// reviewer does after editing, and `cr post` reads §7.2's verbs out of the
// draft itself.
func TestASeverityEditIsObservableInExactlyOneEvent(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"))
	redraft(t)
	assert.Equal(t, []string{"f1:raised:high"}, severitiesIn(t, layout),
		"the raise carries the severity the record was raised at")

	writeDraft(t, layout,
		markerEdit(t, readDraft(t, layout), "f1", `severity="high"`, `severity="low"`))
	shim := ghShimming(t, builtPayload(t))

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)

	assert.Equal(t, []string{"f1:raised:high", "f1:kept:low"}, severitiesIn(t, layout),
		"§7.3.1: the edit reaches the record's single outcome event and creates none of its own")
	require.Len(t, shim.writes(t), 1, "and the review was sent once")

	events, err := finding.TriageEvents(layout, draftOwner, draftRepo)
	require.NoError(t, err)
	for i := range events {
		assert.Truef(t, events[i].Action.Valid(),
			"§7.3.1's five names are the complete vocabulary, and %q is not one",
			events[i].Action)
	}
}

// The same edit made twice leaves the ledger as the first run left it.
//
// §7.3.1 keys an event idempotently and round 8's
// triage-event-key-permits-contradiction holds the action as a value, so a
// second `cr post --confirm` over the same draft rewrites the one outcome
// rather than adding a second — which is what keeps §7.3.4's rate a ratio of
// two counts rather than of one count and a number of runs.
func TestASecondConfirmedRunLeavesTheSeverityEventWhereItWas(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"))
	redraft(t)
	writeDraft(t, layout,
		markerEdit(t, readDraft(t, layout), "f1", `severity="high"`, `severity="low"`))
	shim := ghShimming(t, builtPayload(t))

	for range 2 {
		_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
		require.NoError(t, err)
	}

	assert.Equal(t, []string{"f1:raised:high", "f1:kept:low"}, severitiesIn(t, layout))
	assert.Len(t, shim.writes(t), 2,
		"the idempotence is the ledger's; two confirmed runs are two reviews")
}

// The severity a marker edit moves is the severity the payload is built from,
// so the event and the comment the author receives agree about it.
//
// §7.2 calls the row freely editable, and a run whose event recorded one value
// while the author read another would make §7.3.4's demotion rate a measure of
// something nobody saw.
func TestTheEditedSeverityIsTheOneTheRecordCarries(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"))
	redraft(t)
	writeDraft(t, layout,
		markerEdit(t, readDraft(t, layout), "f1", `severity="high"`, `severity="low"`))

	redraft(t)

	stored := draftedFindings(t, layout)
	require.Len(t, stored, 1)
	assert.Equal(t, finding.SeverityLow, stored[0].Severity,
		"§7.2: the edit is applied to the record")
	assert.Contains(t, readDraft(t, layout), `severity="low"`,
		"and the regenerated marker reads it back out of the record")
	assert.Equal(t, []string{"f1:raised:low"}, severitiesIn(t, layout),
		"§7.3.1's idempotence rewrites the raise in place rather than adding one")
	assert.NotContains(t, strings.Join(severitiesIn(t, layout), " "), "severity-changed",
		"round 8's finding is settled with the field and not with a sixth action")
}
