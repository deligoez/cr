package cli

import (
	"maps"
	"slices"
	"time"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// settled is every record the round's draft was rendered for, each with
// §7.3.1's outcome this run's triage made of it, in the order the records
// arrived.
//
// It is one derivation rather than one per writer. §7.3.1 gives `cr draft` the
// discards and `cr post --confirm` all four outcomes, and two walks of the same
// triage could disagree about a single record — which is exactly the
// contradiction round 8's triage-event-key-permits-contradiction closes at the
// key. Closing it there and reopening it here would be no closure at all.
//
// Each carries §7.3.2's edited mark: whether the body the draft holds differs
// from the one the round's last `cr draft` wrote into it, an agent's rewrite
// that draft preserved included. So only a change made after that draft counts,
// which is the human's. The mark is nil for a record that draft wrote no body
// for, and for a round drafted before cr kept the bodies: nothing then says
// what was written.
func (t *triaged) settled() []finding.Settled {
	out := make([]finding.Settled, 0, len(t.read))
	for _, record := range t.read {
		one := finding.Settled{Record: record, Outcome: t.Outcome(record)}
		if written, found := t.written[record.ID]; found {
			edited := t.Regions[record.ID] != written
			one.Edited = &edited
		}
		out = append(out, one)
	}
	return out
}

// discardedSettlements are the records of those the triage discarded, which
// are the outcomes §7.3.1 has `cr draft` write directly.
func (t *triaged) discardedSettlements() []finding.Settled {
	out := make([]finding.Settled, 0, len(t.Deleted)+len(t.Wrong))
	for _, one := range t.settled() {
		if one.Outcome == finding.OutcomeDiscardedNotHere ||
			one.Outcome == finding.OutcomeDiscardedWrong {
			out = append(out, one)
		}
	}
	return out
}

// newDraftClasses is §7.3.3's report for one round: the classes among the
// round's raised records that the repository's ledger has not held on any
// other occasion.
//
// The read is lock-free per §2.3.2 and is taken before the round's own events
// are written, which is what makes a regeneration answer the way the first run
// did — finding.NewClasses excludes this pull request and round for the same
// reason, so the two guards hold whether or not this read happens to run first.
func newDraftClasses(
	l state.Layout, owner, repo string, pr, round int, records []*finding.Finding,
) ([]string, error) {
	held, err := finding.TriageEvents(l, owner, repo)
	if err != nil {
		return nil, err
	}
	return finding.NewClasses(held, pr, round, raisedInRound(records)), nil
}

// raisedInRound are the round's records a draft has raised: the ones it holds
// in `queued`, and the ones a triage has since moved on to `discarded` or
// `posted`.
//
// A discarded record is kept among them because §7.3.3 is a report about the
// round and not about the draft that happens to be current. A class whose every
// record the reviewer deleted or marked `wrong` was still first seen in this
// round — the ledger's `raised` events and `cr stats`' first_seen both say so —
// and a regeneration that forgot it would overwrite summary.json with a report
// that disagrees with the first run's. The records a draft never raised,
// `duplicate` and `suppressed` ones, stay out: nothing was said about them.
func raisedInRound(records []*finding.Finding) []*finding.Finding {
	raised := make([]*finding.Finding, 0, len(records))
	for _, record := range records {
		switch record.State {
		case finding.StateQueued, finding.StateDiscarded, finding.StatePosted:
			raised = append(raised, record)
		}
	}
	return raised
}

// withSettledForcings is §6.3.2's count per class over the round: forced, the
// count ForceQuestions took over the records the draft holds, joined by every
// record the round raised that a triage has since discarded or posted and that
// an earlier moment of §6.3.1 moved from finding to question.
//
// The count is kept for those records for the reason raisedInRound keeps their
// class: the forcing happened in this round, and a reviewer deleting the block
// afterwards does not undo it. A regeneration that dropped them would overwrite
// summary.json's forced_to_question with a report disagreeing with its own
// forced_records.
func withSettledForcings(forced finding.Forcings, records []*finding.Finding, earlier []string) finding.Forcings {
	counts := make(map[string]int, len(forced))
	for _, row := range forced {
		counts[row.Class] += row.Count
	}
	for _, record := range raisedInRound(records) {
		if record.State == finding.StateQueued || record.Grade != finding.GradeArgued ||
			!slices.Contains(earlier, record.ID) {
			continue
		}
		counts[record.Class]++
	}
	out := make(finding.Forcings, 0, len(counts))
	for _, class := range slices.Sorted(maps.Keys(counts)) {
		out = append(out, finding.Forcing{Class: class, Count: counts[class]})
	}
	return out
}

// triageOccasion is the pull request, round, head and moment one command writes
// its §7.3.1 events for.
func triageOccasion(pr int, round *state.Meta) *finding.TriageOccasion {
	return &finding.TriageOccasion{
		PR: pr, Round: round.Round, Head: round.Head, At: time.Now(),
	}
}

// recordDraftTriage is §7.3.1's events for one `cr draft` run: one `raised`
// event per record it queued, and the outcome event for every record this run's
// triage discarded.
//
// It runs after the draft has been published, so the events describe a document
// that exists — a run refused on the way writes none. The events are keyed per
// §7.3.1, so §7.1.6's regeneration rewrites each one in place rather than
// adding a second, and a reviewer who regenerates five times leaves five
// events rather than twenty-five.
//
// A softening is not written here, and neither is a keep. The record stays
// `queued` and is still open to every verb the next reading of the draft
// admits, so its outcome is not settled until the review is sent: §7.3.1 makes
// `cr post --confirm` the one command that writes an outcome per queued record.
// A discard is the exception because `cr draft` is the command that acts on it —
// the record walks to `discarded` and its waiver is written here.
func recordDraftTriage(
	l state.Layout, owner, repo string, pr int, round *state.Meta,
	queued []*finding.Finding, triage *triaged,
) error {
	on := triageOccasion(pr, round)
	if err := finding.RecordRaised(l, owner, repo, queued, on); err != nil {
		return err
	}
	return finding.RecordOutcomes(l, owner, repo, triage.discardedSettlements(), on)
}

// recordPostTriage is §7.3.1's one outcome event per queued record, as
// `cr post --confirm` owes them.
//
// It MUST be called after the network call has returned successfully, and it is
// the whole of round 8's triage-event-key-permits-contradiction: a call
// rejected per §8.4.2 leaves no outcome event behind, so the reviewer's
// ordinary retry — mark one record `wrong` and run again — writes that
// record's only outcome, instead of a second one contradicting a `kept` from
// the run GitHub refused.
//
// `cr post --reconcile` calls it too, once it has adopted a review as posted,
// over the outcomes the send named in posted.json before its call: a review
// adopted after an unknown outcome is the review a successful call created, and
// both paths write through this one function, so the ledger holds the same
// outcome against each raise whichever of them settled the round.
//
// `cr post` without `--confirm` calls nothing here, per §7.3.1: it changes
// nothing, so it counts nothing.
func recordPostTriage(
	l state.Layout, owner, repo string, pr int, round *state.Meta, settled []finding.Settled,
) error {
	return finding.RecordOutcomes(l, owner, repo, settled, triageOccasion(pr, round))
}
