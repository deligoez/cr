package finding

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// anEvented is one record as a triage event is written from it, carrying every
// field §7.3.1 requires an event to hold.
func anEvented(id string) *Finding {
	return &Finding{
		ID: id, Kind: KindFinding,
		Axis: "correctness", Role: "correctness", Class: "unchecked-error",
		Rule: "no-dropped-error", Grade: GradeCited, Severity: SeverityHigh,
		Summary: "The error is dropped.",
	}
}

// anOccasion is the round every event below is written for.
func anOccasion() *TriageOccasion {
	return &TriageOccasion{
		PR: waiverPR, Round: 3, Head: "0a1b2c3",
		At: time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC),
	}
}

// eventsFor reads the repository's ledger back, keyed by record and action, so
// a test can say what survived without depending on the order of the file.
func eventsFor(t *testing.T, l state.Layout) []TriageEvent {
	t.Helper()
	held, err := TriageEvents(l, waiverOwner, waiverRepo)
	require.NoError(t, err)
	return held
}

// actionsOf names each event as "<record>:<action>", in the order the file
// holds them.
func actionsOf(events []TriageEvent) []string {
	named := make([]string, 0, len(events))
	for i := range events {
		named = append(named, events[i].Record+":"+string(events[i].Action))
	}
	return named
}

// §7.3.1's event carries everything the section names, so a report can be cut
// by class, rule, axis, role or grade without reading findings.ndjson back —
// which it could not do anyway, because §7.3.4's rate is computed across every
// pull request of the repository and each has its own findings.ndjson.
func TestATriageEventCarriesEverythingSection731Requires(t *testing.T) {
	layout := waiverHome(t)
	record := anEvented("f1")
	on := anOccasion()
	require.NoError(t, RecordRaised(layout, waiverOwner, waiverRepo, []*Finding{record}, on))

	held := eventsFor(t, layout)
	require.Len(t, held, 1)
	assert.Equal(t, TriageEvent{
		Record: "f1", Action: ActionRaised,
		Class: "unchecked-error", Axis: "correctness", Role: "correctness",
		Grade: GradeCited, Severity: SeverityHigh, Rule: "no-dropped-error",
		PR: waiverPR, Round: 3, Head: "0a1b2c3", At: on.At,
	}, held[0])
}

// specEventFields is §7.3.1's sentence read independently — class, axis, role,
// grade, rule id when present, the PR, the round, and the head — with the three
// the section's other sentences require beside them: the record the event is
// about, the action it records, and round 8's severity.
//
// It is a second transcription of the same requirement for the reason
// finding_test.go's specFields is one of §6.1's table: the struct-equality
// assertion above catches a field that is declared and never filled, and this
// catches a field that is filled and belongs to no sentence. A field with no
// sentence behind it is a field §7.3.2 and §7.3.4 will not count and nobody
// will notice.
var specEventFields = []string{
	"record", "action", "class", "axis", "role", "grade", "severity",
	"rule", "pr", "round", "head", "at",
}

// The event's wire form carries §7.3.1's fields and no others.
//
// `rule` is written here because the record carries one; §7.3.1 asks for it
// "when present", which is the `omitempty` on the field, and the absent case is
// asserted beside the present one so the key is not simply always there.
func TestTheEventsWireFormIsSection731sFieldSet(t *testing.T) {
	written := anOccasion().event(ActionRaised, anEvented("f1"))
	encoded, err := json.Marshal(written)
	require.NoError(t, err)
	var held map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &held))
	assert.ElementsMatch(t, specEventFields, slices.Collect(maps.Keys(held)),
		"§7.3.1 names what an event carries, and this event carries something else")

	unruled := anEvented("f2")
	unruled.Rule = ""
	encoded, err = json.Marshal(anOccasion().event(ActionRaised, unruled))
	require.NoError(t, err)
	// A fresh map, because json.Unmarshal merges into one it is given and
	// the absent key would otherwise be the first document's still standing.
	absent := map[string]json.RawMessage{}
	require.NoError(t, json.Unmarshal(encoded, &absent))
	assert.NotContains(t, absent, "rule", "§7.3.1 asks for the rule id when present")
}

// §7.3.1's idempotence: a second run of the same command over the same round
// overwrites each event rather than appending one, so §7.1.6's regeneration
// cannot inflate a count.
func TestRewritingAnEventLeavesOneOfIt(t *testing.T) {
	layout := waiverHome(t)
	records := []*Finding{anEvented("f1"), anEvented("f2")}
	for range 3 {
		require.NoError(t,
			RecordRaised(layout, waiverOwner, waiverRepo, records, anOccasion()))
	}

	assert.Equal(t, []string{"f1:raised", "f2:raised"}, actionsOf(eventsFor(t, layout)),
		"§7.3.1: three runs of one round leave one event per record")
}

// Round 8's triage-event-key-permits-contradiction at the ledger: a rejected
// post writes no outcome event, the reviewer re-triages one record, and the
// record ends with exactly one outcome against its single raise.
//
// The rejection is simulated here rather than driven; cli's
// TestARejectedCallMarksNothingPosted drives it through `cr post --confirm`
// against a gh that refuses and then one that accepts. The first outcome is
// written here as `kept`, which is what a run whose network call succeeded
// would have written. It is the second half of the
// finding's reachable path that this asserts: with the action inside the key
// the record would hold both `kept` and `discarded-wrong`, §7.3.2 would report
// two outcomes against one raise, and §7.3.4's demotion rate for the class
// would be inflated on exactly the class the retry touched.
func TestARetriedPostLeavesOneOutcomeAgainstOneRaise(t *testing.T) {
	layout := waiverHome(t)
	record := anEvented("f1")
	on := anOccasion()
	require.NoError(t, RecordRaised(layout, waiverOwner, waiverRepo, []*Finding{record}, on))

	// The rejected call of §8.4.2: nothing follows it, because the
	// outcome events are written after the network call and it did not
	// return one.
	assert.Equal(t, []string{"f1:raised"}, actionsOf(eventsFor(t, layout)),
		"§8.4.2: a rejected call leaves no outcome event behind")

	require.NoError(t, RecordOutcomes(layout, waiverOwner, waiverRepo,
		[]Settled{{Record: record, Outcome: OutcomeKept}}, on))
	require.NoError(t, RecordOutcomes(layout, waiverOwner, waiverRepo,
		[]Settled{{Record: record, Outcome: OutcomeDiscardedWrong}}, on))

	assert.Equal(t, []string{"f1:raised", "f1:discarded-wrong"},
		actionsOf(eventsFor(t, layout)),
		"the later outcome replaces the earlier, and the raise survives both")
}

// The raise is keyed apart from the outcome, and the pull request and the round
// are both in the key: the same record id in another round or another pull
// request is another event, because §7.3.4 counts over the whole repository.
func TestTheKeyIsThePullRequestTheRoundAndTheRecord(t *testing.T) {
	layout := waiverHome(t)
	record := anEvented("f1")
	nextRound := anOccasion()
	nextRound.Round = 4
	otherPR := anOccasion()
	otherPR.PR = waiverPR + 1
	for _, on := range []*TriageOccasion{anOccasion(), nextRound, otherPR} {
		require.NoError(t,
			RecordRaised(layout, waiverOwner, waiverRepo, []*Finding{record}, on))
		require.NoError(t, RecordOutcomes(layout, waiverOwner, waiverRepo,
			[]Settled{{Record: record, Outcome: OutcomeKept}}, on))
	}

	assert.Len(t, eventsFor(t, layout), 6,
		"three occasions, and a raise and an outcome that never overwrite each other")
}

// §7.3.1 calls the seven action names the complete vocabulary its statistics
// are computed from, so an eighth is refused rather than written: §7.3.2 and
// §7.3.4 read the file by name, and an action neither counts would leave a
// raise with no outcome while the file looked full.
func TestTheVocabularyIsSevenNamesAndAnOutcomeIsSixOfThem(t *testing.T) {
	assert.Equal(t,
		[]TriageAction{"raised", "kept", "softened", "discarded-not-here", "discarded-wrong",
			"withdrawn-not-here", "withdrawn-wrong"},
		TriageActions())
	for _, outcome := range []Outcome{
		OutcomeKept, OutcomeSoftened, OutcomeDiscardedNotHere, OutcomeDiscardedWrong,
		OutcomeWithdrawnNotHere, OutcomeWithdrawnWrong,
	} {
		assert.True(t, TriageAction(outcome).Valid(), outcome)
	}
	assert.False(t, TriageAction("hardened").Valid(),
		"§7.2's fifth verb is a record the reviewer kept, not an action of its own")

	layout := waiverHome(t)
	for _, refused := range []Outcome{"hardened", "", Outcome(ActionRaised)} {
		err := RecordOutcomes(layout, waiverOwner, waiverRepo,
			[]Settled{{Record: anEvented("f1"), Outcome: refused}}, anOccasion())
		require.Error(t, err, refused)
		assert.Contains(t, err.Error(), "f1")
	}
	assert.Empty(t, eventsFor(t, layout), "and nothing reached the ledger")
}

// A run with nothing to write touches nothing, so a command that raised no
// record does not create a ledger the repository had none of.
func TestAnEmptyRunWritesNoLedger(t *testing.T) {
	layout := waiverHome(t)
	require.NoError(t, RecordRaised(layout, waiverOwner, waiverRepo, nil, anOccasion()))
	require.NoError(t, RecordOutcomes(layout, waiverOwner, waiverRepo, nil, anOccasion()))
	assert.Empty(t, eventsFor(t, layout))
}
