package finding

import (
	"fmt"
	"slices"
	"time"

	"github.com/deligoez/cr/internal/state"
)

// TriageAction is one of §7.3.1's seven action names, which the section calls
// the complete vocabulary its statistics are computed from: the raise, and the
// six outcomes Outcome already holds.
//
// It is a type of its own rather than a fifth Outcome because the raise is not
// an outcome. §7.3.4 divides by the raises and counts two of the outcomes into
// the numerator, so a vocabulary that let `raised` stand where an outcome is
// expected would make the rate divide by a number that includes itself.
type TriageAction string

// ActionRaised is §7.3.1's fifth action: a record `cr draft` queued, written
// once per record and never replaced by an outcome.
const ActionRaised TriageAction = "raised"

// TriageActions returns §7.3.1's seven action names, the raise first and then
// the six outcomes in the order §7.3.2 reports them. The result is a copy, so
// a caller can neither widen the vocabulary nor reorder it.
func TriageActions() []TriageAction {
	return []TriageAction{
		ActionRaised,
		TriageAction(OutcomeKept),
		TriageAction(OutcomeSoftened),
		TriageAction(OutcomeDiscardedNotHere),
		TriageAction(OutcomeDiscardedWrong),
		TriageAction(OutcomeWithdrawnNotHere),
		TriageAction(OutcomeWithdrawnWrong),
	}
}

// Valid reports whether the action is one of the seven.
func (a TriageAction) Valid() bool {
	return slices.Contains(TriageActions(), a)
}

// TriageEvent is one entry of a repository's triage.ndjson: what happened to
// one record on one round, carrying everything §7.3.1 requires of an event —
// class, axis, role, grade, rule id when present, the pull request, the round,
// and the head.
//
// The record id is carried beside them because the key needs it, and the
// moment because §7.3.3 reports every class first seen in a round: a round
// index is per pull request and means nothing across two of them, so the only
// ordering the file has across the repository is the timestamp.
type TriageEvent struct {
	// Record is the id of the record the event is about.
	Record string `json:"record"`
	// Action is which of §7.3.1's five this event records. On an outcome
	// event it is a value and never part of the key, per round 8's
	// triage-event-key-permits-contradiction.
	Action TriageAction `json:"action"`
	// Class is the record's defect class, which §7.3.2 reports per and
	// §7.3.4 computes its rate over.
	Class string `json:"class"`
	// Axis, Role and Grade are the record's, recorded so a report can be
	// cut by any of them without reading findings.ndjson back.
	Axis  string `json:"axis"`
	Role  string `json:"role"`
	Grade Grade  `json:"grade"`
	// Severity is the record's severity as it stood when the event was
	// written, which is round 8's unnameable-triage-event settled the way
	// that finding's second option settles it.
	//
	// §7.2's marker table calls `severity` freely editable and says the
	// edit is recorded as a triage event, while §7.3.1 closes the action
	// vocabulary at five names and allows `cr post --confirm` exactly one
	// outcome event per queued record. A record whose only draft edit is a
	// severity change therefore had no event it could be written as: a
	// sixth action would break the closed vocabulary, and a second outcome
	// event would break the exactly-one rule.
	//
	// Carrying the value on every event closes both at once. The edit is
	// recorded on the record's single outcome event rather than as an
	// event of its own, because §7.2's edits are applied before that event
	// is written — so the severity an outcome carries is the severity the
	// reviewer settled on, and the raise beside it still carries the one
	// the record was raised at. What changed is then readable as the
	// difference between two events the section already required.
	Severity Severity `json:"severity"`
	// Rule is the rule id when a rule of §2.6 produced the record, which
	// §7.3.2 reports per, and absent when none did.
	Rule string `json:"rule,omitempty"`
	// Edited is §7.3.2's mark on a `kept` or `softened` event: true when
	// the body posted differs from the body the round's last `cr draft`
	// rendered for the record, its rendered.json entry, and false when it
	// does not. It is absent on every other action, and on an event written
	// before cr recorded it, which is why it is a pointer: an absent mark is
	// not the same answer as an unedited body.
	Edited *bool `json:"edited,omitempty"`
	// PR, Round and Head are the occasion the event was written for.
	PR    int    `json:"pr"`
	Round int    `json:"round"`
	Head  string `json:"head"`
	// At is when it was written, in UTC.
	At time.Time `json:"at"`
}

// TriageOccasion is the pull request, round, head and moment one command writes
// its triage events for.
type TriageOccasion struct {
	PR    int
	Round int
	Head  string
	At    time.Time
}

// event is one event about a record, written on this occasion.
func (o *TriageOccasion) event(action TriageAction, record *Finding) TriageEvent {
	return TriageEvent{
		Record: record.ID, Action: action,
		Class: record.Class, Axis: record.Axis, Role: record.Role,
		Grade: record.Grade, Severity: record.Severity, Rule: record.Rule,
		PR: o.PR, Round: o.Round, Head: o.Head, At: o.At.UTC(),
	}
}

// triageKey is the key §7.3.1 writes an event idempotently under, as round 8's
// triage-event-key-permits-contradiction settles it: the pull request, the
// round, and the record id, with the action held as a value.
//
// The raise is keyed apart from the outcome, and that is the whole of the
// distinction. Two outcome events for one record in one round are a
// contradiction — the reachable path is the ordinary retry after §8.4.2, where
// a rejected call leaves the records queued and the reviewer re-triages one of
// them — so the later outcome replaces the earlier rather than joining it, and
// §7.3.2's report can never show a class's outcomes exceeding its raises. The
// raise is a different fact about the same record and survives both.
type triageKey struct {
	record string
	pr     int
	round  int
	raised bool
}

func (e *TriageEvent) key() triageKey {
	return triageKey{
		record: e.Record, pr: e.PR, round: e.Round,
		raised: e.Action == ActionRaised,
	}
}

// Settled is one record and the outcome §7.2's triage settled on for it, which
// is what an outcome event is written from.
type Settled struct {
	// Record is the record the outcome belongs to.
	Record *Finding
	// Outcome is one of §7.3.1's six outcome actions.
	Outcome Outcome
	// Edited is whether the body posted differs from the record's
	// rendered.json entry, per §7.3.2, and nil when it is not known. It is
	// carried onto a `kept` or `softened` event and onto no other.
	Edited *bool
}

// RecordRaised writes one `raised` event per record, per §7.3.1's rule for
// `cr draft`: a record it queues is a record raised against the author, whether
// or not the reviewer later keeps it.
//
// A second run of the same round overwrites each event rather than appending
// one, so §7.1.6's regeneration cannot inflate §7.3.4's denominator.
//
// A record the pull request already holds a `raised` event for is not raised
// again, in any round. §9.3.4 carries a queued record into the next round, and
// the round is part of the event's key, so without this the carried record
// would be raised twice and §7.3.4's rate would divide by a record counted
// twice. Before v0.7.0 no record could reach a second round's draft, so the
// rule changes nothing that happened earlier.
func RecordRaised(
	l state.Layout, owner, repo string, raised []*Finding, on *TriageOccasion,
) error {
	held, err := TriageEvents(l, owner, repo)
	if err != nil {
		return err
	}
	already := make(map[string]bool)
	for i := range held {
		if held[i].Action == ActionRaised && held[i].PR == on.PR && held[i].Round != on.Round {
			already[held[i].Record] = true
		}
	}
	written := make([]TriageEvent, 0, len(raised))
	for _, record := range raised {
		if already[record.ID] {
			continue
		}
		written = append(written, on.event(ActionRaised, record))
	}
	return writeTriageEvents(l, owner, repo, written)
}

// RecordOutcomes writes one outcome event per settled record, per §7.3.1.
//
// `cr draft` calls it for the records it discards at draft time, and
// `cr post --confirm` for every queued record once the network call has
// returned — after it, so a call rejected per §8.4.2 leaves no outcome event
// behind and the retry's outcome is the only one the file ever holds.
//
// An action outside §7.3.1's vocabulary is refused rather than written. The
// section calls the five names complete, and §7.3.2 and §7.3.4 read the file by
// name: a sixth would be counted by neither and would silently leave a raise
// with no outcome.
func RecordOutcomes(
	l state.Layout, owner, repo string, settled []Settled, on *TriageOccasion,
) error {
	written := make([]TriageEvent, 0, len(settled))
	for _, one := range settled {
		action := TriageAction(one.Outcome)
		if action == ActionRaised || !action.Valid() {
			return fmt.Errorf(
				"record %s: %q is not one of §7.3.1's six outcome actions",
				one.Record.ID, one.Outcome)
		}
		event := on.event(action, one.Record)
		if one.Outcome == OutcomeKept || one.Outcome == OutcomeSoftened {
			event.Edited = one.Edited
		}
		written = append(written, event)
	}
	return writeTriageEvents(l, owner, repo, written)
}

// TriageEvents reads one repository's triage events, lock-free per §2.3.2.
func TriageEvents(l state.Layout, owner, repo string) ([]TriageEvent, error) {
	return state.ReadTriageEventRecords[TriageEvent](l, owner, repo)
}

// writeTriageEvents publishes the events under the ledger's lock, leaving every
// event they do not share a key with byte for byte.
//
// A run with nothing to write touches nothing, so a command that raised no
// record does not create a ledger for the repository.
func writeTriageEvents(l state.Layout, owner, repo string, written []TriageEvent) error {
	if len(written) == 0 {
		return nil
	}
	return state.UpdateTriage(l, owner, repo, func(existing []TriageEvent) []TriageEvent {
		return mergeTriageEvents(existing, written)
	})
}

// mergeTriageEvents carries every existing event through in the order the file
// held it, overwrites in place the one a written event shares a key with, and
// appends the written events no existing one held.
func mergeTriageEvents(existing, written []TriageEvent) []TriageEvent {
	at := make(map[triageKey]int, len(existing))
	merged := make([]TriageEvent, 0, len(existing)+len(written))
	for i := range existing {
		at[existing[i].key()] = len(merged)
		merged = append(merged, existing[i])
	}
	for i := range written {
		if held, found := at[written[i].key()]; found {
			merged[held] = written[i]
			continue
		}
		at[written[i].key()] = len(merged)
		merged = append(merged, written[i])
	}
	return merged
}
