package probe

// Proven is §5.3.5's result condition read off a stored mutation record rather
// than off the run that produced it.
//
// It is Reproduces' twin on the other ladder, and it exists for the reason
// Reproduces does: §6.2.1 names "the referenced probe record" as grading's
// input, and a finding is graded long after the run that produced that record
// has gone. Proves asks the same question of an Outcome, which only Decide can
// make, and that is the right shape at the moment the experiment finishes —
// but no Outcome survives into probes.ndjson, so a grader has the record and
// nothing else.
//
// Reading the record loses nothing §5.1.7 established. §5.5.1 makes the stored
// `result` immutable and Decide is what wrote it, so a voided probe arrives
// here carrying `error` and never `no-test-failed`; the override has already
// had its say by the time this can be asked.
//
// The baseline is deliberately not read here. §5.3.5 has two conditions and
// this is the first; the second is §5.2.5's verdict, which only a Baseline can
// answer for the run the probe was actually measured against, and Establishes
// is where the two are put together.
func Proven(record *Record) bool {
	return record.Kind == Mutation && record.Result == resultNoTestFailed
}

// Establishes is §6.2's `probed` row asked of one stored probe record: the head
// matches, and the result supports the claim per §5.3 and §5.4.
//
// It is one function over both kinds because §6.2's row is one sentence over
// both. §5.3.5 and §5.4.4 state different conditions, and the difference is
// kept where the sections keep it — Proven and Reproduces read the two
// vocabularies, and neither can be reached with the other kind's record — but
// which of them a grader asks is not a decision a grader should be making, and
// a caller that chose wrongly would grade a gap probe on the mutation ladder.
//
// The head is asked first, per §5.5.3: a probe from another head must not grade
// a finding in this round, whatever it established about the tree it ran
// against.
//
// baseline and claim are the unforgeable halves. A Baseline exists only where
// §5.2.6 admitted a stored run and carries §5.2.5's verdict on that run; a
// ClaimMapping exists only where §4.1.6's stored mapping was asked. Neither is
// a field the agent writes and neither can be assembled out of a value a caller
// preferred, so no record can talk its way to `probed`.
//
// A record referencing no probe reaches this as nil, which is the same answer
// as a probe that supports nothing: §6.2's `argued` row is "neither of the
// above", and a record with no experiment behind it has not met the first.
// Whether naming a probe that does not exist is refused outright is `cr
// record`'s question, asked before any grade.
//
// span is the record's own anchor, and §6.2.2's binding is asked right after
// the head: a probe supports a record only when its target falls within that
// range on the same path, and never for a LEFT anchor. A probe that exists at
// this head and misses the range supports nothing, and the record stays argued
// as §5.4.4 and §5.4.5 leave a record whose probe does not support it.
func Establishes(record *Record, head string, baseline Baseline, claim ClaimMapping, span Span) bool {
	if record == nil || !record.Grades(head) || !span.Holds(record.Target) {
		return false
	}
	switch record.Kind {
	case Mutation:
		return Proven(record) && baseline.Passed()
	case Gap:
		return Supports(record, baseline, claim)
	}
	// A kind outside §5.5's two, which CheckResult refuses at the write.
	// Reached only by a record cr did not store, and answered the way
	// §6.2's `argued` row answers everything it does not recognise.
	return false
}
