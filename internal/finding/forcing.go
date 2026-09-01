package finding

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
// and a record the agent wrote as a question was not forced into one.
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
