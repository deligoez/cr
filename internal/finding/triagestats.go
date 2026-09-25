package finding

import (
	"fmt"
	"maps"
	"slices"
)

// TriageCounts is §7.3.2's seven counts over one subject's triage events:
// raised, kept, softened, discarded as `not-here` and as `wrong`, and
// withdrawn as `not-here` and as `wrong`.
//
// The seven are §7.3.1's complete action vocabulary and nothing else, so a
// subject's counts are a partition of its events rather than a selection from
// them. That is what lets §7.3.4 divide by Raised and know the denominator is
// the whole of what was raised.
type TriageCounts struct {
	// Raised is §7.3.1's `raised`: a record `cr draft` queued.
	Raised int `json:"raised"`
	// Kept, Softened, DiscardedNotHere and DiscardedWrong are the four
	// outcomes triage settles, in the order §7.3.2 names them.
	Kept             int `json:"kept"`
	Softened         int `json:"softened"`
	DiscardedNotHere int `json:"discarded_not_here"`
	DiscardedWrong   int `json:"discarded_wrong"`
	// WithdrawnNotHere and WithdrawnWrong are §9.6.2's retractions. Each
	// replaces the `kept` its record was posted under (§7.3.1), so a raise
	// still has one outcome and the seven still partition the events.
	WithdrawnNotHere int `json:"withdrawn_not_here"`
	WithdrawnWrong   int `json:"withdrawn_wrong"`
	// Unedited is how many of the kept and softened were posted unedited:
	// their event carries `edited: false`. It is the measure of whether a
	// human can send a block without editing it.
	Unedited int `json:"unedited"`
	// EditUnknown is how many of the kept and softened carry no `edited`
	// mark at all, having been written before cr recorded it. They are
	// counted apart rather than as unedited, since nothing says they were.
	EditUnknown int `json:"edit_unknown"`
}

// count adds one action to the tally, and refuses one outside §7.3.1's five.
//
// The refusal is not defensive noise. triage.ndjson is a file under `~/.cr`
// that a user can open and edit, and §7.3.1 calls the five names the complete
// vocabulary the statistics are computed from — so an action this switch does
// not know would be counted by no arm, silently shrinking a class's raises and
// inflating its demotion rate. A report that is wrong about a class is exactly
// what the trust economy spends its budget on, so the read stops instead.
func (c *TriageCounts) count(event *TriageEvent) error {
	if err := c.countAction(event.Action); err != nil {
		return err
	}
	if event.Action != TriageAction(OutcomeKept) && event.Action != TriageAction(OutcomeSoftened) {
		return nil
	}
	switch {
	case event.Edited == nil:
		c.EditUnknown++
	case !*event.Edited:
		c.Unedited++
	}
	return nil
}

// countAction adds the event's action to its one count of the seven.
func (c *TriageCounts) countAction(action TriageAction) error {
	switch action {
	case ActionRaised:
		c.Raised++
	case TriageAction(OutcomeKept):
		c.Kept++
	case TriageAction(OutcomeSoftened):
		c.Softened++
	case TriageAction(OutcomeDiscardedNotHere):
		c.DiscardedNotHere++
	case TriageAction(OutcomeDiscardedWrong):
		c.DiscardedWrong++
	case TriageAction(OutcomeWithdrawnNotHere):
		c.WithdrawnNotHere++
	case TriageAction(OutcomeWithdrawnWrong):
		c.WithdrawnWrong++
	default:
		return fmt.Errorf(
			"triage event: %q is not one of §7.3.1's seven action names", action)
	}
	return nil
}

// ClassTriage is §7.3.2's counts for one defect class.
type ClassTriage struct {
	// Class is the class the counts are over.
	Class string `json:"class"`
	TriageCounts
}

// RuleTriage is §7.3.2's counts for one rule of §2.6.
type RuleTriage struct {
	// Rule is the rule id the counts are over.
	Rule string `json:"rule"`
	TriageCounts
}

// TriageReport is §7.3.2's report over one repository's triage ledger: the
// five counts per class and per rule.
//
// Both cuts come from one pass over one file, because they have to agree. A
// class's counts and the counts of the rules that produced it are two views of
// the same events, and computing them from two reads would let a write landing
// between them show a class one raise ahead of its own rule.
type TriageReport struct {
	// Classes are the per-class counts, ordered by class.
	Classes []ClassTriage `json:"classes"`
	// Rules are the per-rule counts, ordered by rule id. An event that
	// names no rule is in Classes and in no row here: §7.3.1 carries the
	// rule id only when a rule of §2.6 produced the record, and a row
	// gathering the rest under an empty id would report a rule that does
	// not exist.
	Rules []RuleTriage `json:"rules"`
}

// Tally counts one repository's triage events per class and per rule.
//
// The order is by name rather than by first appearance, and that is §2.1.1
// rather than a preference: the same ledger has to report the same way twice,
// and the ledger's own order is the order rounds happened to be drafted in.
func Tally(events []TriageEvent) (TriageReport, error) {
	byClass, byRule := map[string]*TriageCounts{}, map[string]*TriageCounts{}
	at := func(held map[string]*TriageCounts, key string) *TriageCounts {
		if counts, found := held[key]; found {
			return counts
		}
		held[key] = &TriageCounts{}
		return held[key]
	}
	for i := range events {
		if err := at(byClass, events[i].Class).count(&events[i]); err != nil {
			return TriageReport{}, fmt.Errorf("record %s: %w", events[i].Record, err)
		}
		if events[i].Rule == "" {
			continue
		}
		if err := at(byRule, events[i].Rule).count(&events[i]); err != nil {
			return TriageReport{}, fmt.Errorf("record %s: %w", events[i].Record, err)
		}
	}

	report := TriageReport{
		Classes: make([]ClassTriage, 0, len(byClass)),
		Rules:   make([]RuleTriage, 0, len(byRule)),
	}
	for _, class := range slices.Sorted(maps.Keys(byClass)) {
		report.Classes = append(report.Classes,
			ClassTriage{Class: class, TriageCounts: *byClass[class]})
	}
	for _, rule := range slices.Sorted(maps.Keys(byRule)) {
		report.Rules = append(report.Rules,
			RuleTriage{Rule: rule, TriageCounts: *byRule[rule]})
	}
	return report, nil
}
