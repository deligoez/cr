package finding

// Outcome is what §7.2's triage, or §9.6.2's withdrawal, made of one record,
// named by §7.3.1's six outcome actions.
//
// It is the record's contribution to §7.3's statistics, and nothing more: the
// triage event that carries it to triage.ndjson is written by the commands
// §7.3.1 names, and this is the value such an event holds.
type Outcome string

// The six outcome actions of §7.3.1: four ways a queued record can leave the
// reviewer's hands, and two ways a posted one can be taken back.
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
	// OutcomeWithdrawnWrong is a posted record the reviewer retracted with
	// `cr withdraw … wrong` (§9.6.2): the author read it, and it was false.
	OutcomeWithdrawnWrong Outcome = "withdrawn-wrong"
	// OutcomeWithdrawnNotHere is one retracted with `cr withdraw …
	// not-here`: true, but not worth the comment it became.
	OutcomeWithdrawnNotHere Outcome = "withdrawn-not-here"
)

// CountsAgainstClass reports whether §7.3.4's demotion rate counts the outcome
// in its numerator: `discarded-wrong`, `withdrawn-wrong` and `softened` do, and
// neither `not-here` ever does, because §7.2 makes it the ordinary case of a
// true finding not worth saying rather than evidence the class is imprecise.
func (o Outcome) CountsAgainstClass() bool {
	return o == OutcomeDiscardedWrong || o == OutcomeWithdrawnWrong || o == OutcomeSoftened
}

// NotHere reports whether the outcome says the record was true and not worth
// saying, which §7.3.6's volume rate counts: a deletion before posting or a
// withdrawal after it.
func (o Outcome) NotHere() bool {
	return o == OutcomeDiscardedNotHere || o == OutcomeWithdrawnNotHere
}
