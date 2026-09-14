package finding

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// ForceQuestion applies §6.3.1 to one record: a record graded `argued` is
// forced to `kind: question`. It reports whether this application is what moved
// it, which is the forcing §6.3.2 counts.
//
// This is invariant 4, and the invariant names where it lives as much as what
// it says: "the forcing happens in cr, not in the prompt, so no agent can talk
// its way past it". A role instruction asking for restraint is advice a model
// weighs against everything else in its context; this is a field assignment,
// and there is no wording that reaches it. §6.3.3 closes the other doors — no
// flag, configuration setting, environment variable, profile field, or role
// instruction disables it — and §2.7 keeps the forcing off the settings table
// so no layer has a key to turn.
//
// P2 and P3 are what it buys. `argued` is §6.2's "neither of the above": no
// experiment cr ran, and no location cr resolved outside the record's own unit.
// A record with nothing behind it may still be right and may still be worth
// sending, but it may not be sent as an assertion — so cr re-shapes the
// uncertainty into a question rather than suppressing the record, and §8.1.4
// prepends a built-in line naming the register and the grade so the author is
// told which they are reading.
//
// §6.3.1 asks for the forcing at three moments — record time, draft time, and
// post time immediately before the payload is built — and the repetition is not
// belt and braces. §6.2.1's inputs move within a round: §4.1.6 replaces the
// mapping and §3.3.1 clears it, so Regrade can lower a record to `argued` after
// it was recorded as a finding. A forcing applied only at record time would
// leave that record in the assertion register for the rest of the round, and
// the moment that matters most is the last one, because it is the only one that
// sees the payload the author will actually receive.
//
// It is idempotent, which is what lets it be applied three times without the
// second application meaning anything different from the first, and it reports
// false for a record already in the question register: §6.3.2 counts forcings,
// and a record the agent wrote as a question was not forced into one. Nor, at
// the second application, is a record the first one moved — which is why
// ForceQuestions is told the ids an earlier moment moved.
//
// Nothing here raises the register. A record graded `probed` or `cited` keeps
// the kind the agent wrote, question included — §6.2 lets those two assert and
// does not require them to, and an agent that chose to ask is making the
// judgement §2.1.3 gives it.
func ForceQuestion(record *Finding) bool {
	if record.Grade != GradeArgued || record.Kind == KindQuestion {
		return false
	}
	record.Kind = KindQuestion
	return true
}

// Forcing is one row of §6.3.2's report: a defect class, and how many of the
// round's records §6.3.1 moved from finding to question under it.
type Forcing struct {
	// Class is §6.1's kebab-case defect class.
	Class string `json:"class"`
	// Count is how many records of that class §6.3 forced.
	Count int `json:"count"`
}

// Forcings is §6.3.2's report over one round: the forcing's count per class, by
// class ascending.
//
// What it counts is every record §6.3.1 moved from finding to question at any of
// the round's moments, and never a record graded `argued` that its role already
// wrote as a question: that one was asked, not forced, and §6.3.2 makes the
// forcing visible so that a reader can tell the two apart. Counting the grade
// alone reported release QA's unmapped-unit question, written as a question by
// its role, as forced.
//
// The moves are counted across moments rather than within one call, because
// §6.3.1 applies the forcing three times and stores what it moved: by draft
// time the records moved at record time already read as questions, and a
// report counting one call's moves would read zero at exactly the moments a
// human sees it. So ForceQuestions takes the ids an earlier moment moved, which
// the round summary keeps, and counts them beside the ones it moves itself.
//
// The order is by class rather than by count or by arrival, so the same round
// renders the same report twice and a stored summary diffs cleanly.
type Forcings []Forcing

// Total is how many records the forcing holds across every class.
func (f Forcings) Total() int {
	total := 0
	for _, forced := range f {
		total += forced.Count
	}
	return total
}

// Disclosure is §6.3.2's report, one of the seven §11.1 exempts from `--quiet`,
// in the shape cap.go's HonestyDisclosure fixes.
//
// It is printed at zero as well, for the reason Overlaps.Disclosure gives and
// one of its own. §11.1 exempting this report is only worth anything if the
// report is always there: a line that appeared only when something was forced
// would leave a reviewer unable to tell a round where nothing was argued from a
// round where the forcing never ran — and the second is a round in which every
// weak finding reached the author as an assertion.
func (f Forcings) Disclosure() string {
	if len(f) == 0 {
		return "§6.3: 0 records forced to question"
	}
	named := make([]string, 0, len(f))
	for _, forced := range f {
		named = append(named, fmt.Sprintf("%s %d", forced.Class, forced.Count))
	}
	return fmt.Sprintf("§6.3: %d records forced to question — %s",
		f.Total(), strings.Join(named, ", "))
}

// ArguedAssertionError reports a record §6.2 graded `argued` that is still in
// the assertion register, which §6.3.3 refuses.
//
// The cli layer maps it onto exit code 1, which §6.3.3 fixes. Reaching it means
// the forcing did not hold: §6.3.1 applies it at record time, at draft time and
// at post time immediately before the payload is built, so a record that is
// both `argued` and a finding got past all three.
type ArguedAssertionError struct {
	// Record is the record id §6.3.3 requires the refusal to name.
	Record string
}

func (e *ArguedAssertionError) Error() string {
	return fmt.Sprintf(
		"%s is graded argued and written as %s, and §6.3.3 admits no flag, setting, "+
			"environment variable, profile field, or role instruction that overrides the forcing: "+
			"an argued record rests on no experiment cr ran and no location cr resolved outside "+
			"its own unit, so it may be asked and never asserted",
		e.Record, KindFinding)
}

// RefuseArguedAssertion is §6.3.3's rejection: no record graded `argued` leaves
// cr in the assertion register, and the refusal names the record id.
//
// It is a proof rather than a second application of the rule. ForceQuestions
// has already moved every such record, so a caller running both should never
// see this error — and that is the point. Invariant 4 is a claim about what
// reaches the author, and a claim resting on one function having been called
// correctly is a claim that fails silently the day that function stops working.
// Checking the records that are actually about to be written turns it into a
// property of the payload.
//
// The first offender stops the run rather than a list being gathered. §6.3.3
// gives no partial outcome to report: nothing may be posted while one argued
// assertion is in the batch, so the run ends at the first one with its id
// named, and correcting it brings the next into view.
func RefuseArguedAssertion(records []*Finding) error {
	for _, record := range records {
		if record.Grade == GradeArgued && record.Kind != KindQuestion {
			return &ArguedAssertionError{Record: record.ID}
		}
	}
	return nil
}

// ForceQuestions applies §6.3.1 to every record of a round and reports §6.3.2's
// count per class, together with the ids of the records this application moved.
//
// earlier is the ids an earlier moment of the same round's forcing moved. A
// record among them still graded `argued` is counted although it already reads
// as a question, and a record this application moves is counted because it
// did; a record graded `argued` that is neither was written as a question by
// its role, and is not counted. The ids moved here are returned so the caller
// can keep them for the moments after it.
//
// The two are one call because they are one obligation seen from two sides.
// §6.3.1 forces and §6.3.2 makes the forcing visible, and a caller that could
// force without counting would apply the rule and tell nobody — which is the
// failure §6.3.2 exists to prevent, since a question is otherwise
// indistinguishable from a question the agent chose to ask.
func ForceQuestions(records []*Finding, earlier []string) (forced Forcings, moved []string) {
	counts := make(map[string]int, len(records))
	moved = make([]string, 0, len(records))
	for _, record := range records {
		switch {
		case ForceQuestion(record):
			moved = append(moved, record.ID)
		case record.Grade != GradeArgued || !slices.Contains(earlier, record.ID):
			continue
		}
		counts[record.Class]++
	}
	forced = make(Forcings, 0, len(counts))
	for _, class := range slices.Sorted(maps.Keys(counts)) {
		forced = append(forced, Forcing{Class: class, Count: counts[class]})
	}
	return forced, moved
}

// Withdrawn is §3.6.6's report over one round, in Forcings' shape: per class,
// how many of the round's records rest on a claim whose note no longer stands
// and are therefore held in the question register.
//
// It is a report of its own rather than rows of §6.3.2's, because the cause is
// a different one and §6.3.2 counts a grade. A record resting on a withdrawn
// note keeps the grade §6.2 computed for it — §6.2.1 closes the grade's inputs
// and the context store is not among them — while the register is taken away
// from it, since hearsay a reviewer has withdrawn is no provenance to assert on.
// It counts the records the rule holds rather than the moves one run made, for
// the reason Forcings gives: the rule is applied at draft time and again at post
// time, and the second application moves nothing.
type Withdrawn []Forcing

// Disclosure is the report for a terminal, printed at zero for the reason
// Forcings.Disclosure is: a line that appeared only when something was held
// would leave a reader unable to tell a round with no withdrawn note from a
// round where the rule never ran.
func (w Withdrawn) Disclosure() string {
	if len(w) == 0 {
		return "§3.6.6: 0 records resting on a withdrawn note held as question"
	}
	named := make([]string, 0, len(w))
	for _, held := range w {
		named = append(named, fmt.Sprintf("%s %d", held.Class, held.Count))
	}
	return fmt.Sprintf("§3.6.6: %d records resting on a withdrawn note held as question — %s",
		Forcings(w).Total(), strings.Join(named, ", "))
}

// ForceWithdrawn holds in the question register every record whose claim rests
// on a note that no longer stands, and reports §3.6.6's count per class.
//
// withdrawn is the set of the round's claim ids whose note the context store
// reports retracted or no longer holds, as note.StandingOf answers it at the
// moment the caller asks. It is handed in because internal/note imports this
// package, and because the standing is read from the store every time rather
// than stamped onto a record: a note retracted after `cr record` bites the next
// draft and the next post of the same round, which is the only round v0.1 is
// sure to have.
//
// Nothing here raises the register or touches the grade, and the record is
// retained rather than dropped, so the decision stays auditable and a reviewer
// still reads what was raised — as a question.
func ForceWithdrawn(records []*Finding, withdrawn map[string]bool) Withdrawn {
	counts := make(map[string]int, len(records))
	for _, record := range records {
		if !withdrawn[record.Claim] {
			continue
		}
		record.Kind = KindQuestion
		counts[record.Class]++
	}
	held := make(Withdrawn, 0, len(counts))
	for _, class := range slices.Sorted(maps.Keys(counts)) {
		held = append(held, Forcing{Class: class, Count: counts[class]})
	}
	return held
}
