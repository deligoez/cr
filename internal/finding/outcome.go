package finding

// Outcome is what §7.2's triage made of one queued record, named by §7.3.1's
// four outcome actions.
//
// It is the record's contribution to §7.3's statistics, and nothing more: the
// triage event that carries it to triage.ndjson is written by the commands
// §7.3.1 names, and this is the value such an event holds.
type Outcome string

// The four outcome actions of §7.3.1, one per way a queued record can leave
// the reviewer's hands.
const (
	// OutcomeKept is a block left as rendered, or with its body edited.
	// Either way the record is posted as the reviewer left it.
	OutcomeKept Outcome = "kept"
	// OutcomeSoftened is a block whose marker the reviewer changed from
	// `kind=finding` to `kind=question`.
	OutcomeSoftened Outcome = "softened"
	// OutcomeDiscardedNotHere is a block the reviewer deleted: true, but
	// not worth a comment on this pull request.
	OutcomeDiscardedNotHere Outcome = "discarded-not-here"
	// OutcomeDiscardedWrong is a block whose marker the reviewer set to
	// `disposition=wrong`: a false positive.
	OutcomeDiscardedWrong Outcome = "discarded-wrong"
)

// CountsAgainstClass reports whether §7.3.4's demotion rate counts the outcome
// in its numerator: `discarded-wrong` and `softened` do, and `not-here` never
// does, because §7.2 makes it the ordinary case of a true finding not worth
// saying rather than evidence the class is imprecise.
func (o Outcome) CountsAgainstClass() bool {
	return o == OutcomeDiscardedWrong || o == OutcomeSoftened
}
