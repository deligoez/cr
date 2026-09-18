package probe

import "fmt"

// setting is §2.7's key for §5.6.4's cap, named here rather than at the call
// site so the refusal, the disclosure and the lookup all spell it once.
const setting = "probe.max_per_round"

// RoundCap is §5.6.4's decision over one round's probe executions: how many the
// round has already run, the cap they were measured against, and nothing else.
//
// Count is what the round has run, never what it is about to run, and Ran is
// how the value after a run is reached. The two are kept apart because they
// answer different questions at different moments — Err is asked before the
// sandbox is touched, and Disclosure after the record has landed — and a single
// mutable counter would have to be right about which moment it was in.
//
// There is deliberately no shape here that fits a run in anyway. §5.6.4 is one
// rule written in two halves — the cap MUST cap, and being hit MUST be reported
// rather than silently applied — and the failure it guards against is a cr that
// quietly stops probing. §1.6.2's comment cap is the same rule about a
// different resource and CommentCap is built the same way: cr reports the
// number and refuses, and raising the cap is the user's decision through §2.7.
type RoundCap struct {
	// Count is how many probe records this round has already recorded.
	// §2.3.3 stamps `round` on every one, so the question is answered
	// from probes.ndjson itself rather than from a counter that could
	// disagree with the file it summarises.
	Count int
	// Max is the resolved probe.max_per_round of §2.7, whose built-in
	// default is 10. It is passed in rather than read here, because §2.7
	// resolves it across five layers and a second path to the value is a
	// second answer.
	Max int
}

// RoundCapFor measures one round's recorded probes against probe.max_per_round.
//
// Records from other rounds are not counted, and that is §5.6.4's own word:
// the cap is per round. §5.5.1 keeps every earlier round's records on disk and
// §5.5.3 already withdraws their standing in this one, so counting them would
// let a pull request's third round refuse the first probe it asked for.
func RoundCapFor(stored []Record, round, maxPerRound int) RoundCap {
	ran := 0
	// Indexed rather than ranged by value: a record carries the whole
	// patch and an output tail, and only its round is read here.
	for i := range stored {
		if stored[i].Round == round {
			ran++
		}
	}
	return RoundCap{Count: ran, Max: maxPerRound}
}

// Ran is the cap as it stands once one more probe has been recorded.
//
// It is what a run discloses, because the number the reader needs is how many
// probes the round has now spent — not how many it had spent before the run
// they are reading the result of.
func (c RoundCap) Ran() RoundCap {
	return RoundCap{Count: c.Count + 1, Max: c.Max}
}

// Reached reports that the round has spent its budget: the cap is the last
// execution that fits, so probe.max_per_round 10 lets a round run ten probes
// and refuses the eleventh.
//
// It is the one comparison, in one place, because Disclosure and Err must never
// disagree about it.
func (c RoundCap) Reached() bool { return c.Count >= c.Max }

// Disclosure is the §11.1 honesty disclosure of the probe cap, printed whatever
// `--quiet` says.
//
// A suppressed cap report is §5.6.4's "silently applied" wearing a different
// hat: the experiments the round can no longer run would go unmentioned, which
// is the outcome the clause exists to prevent, reached by way of an output flag
// rather than by a silent stop.
func (c RoundCap) Disclosure() string {
	if c.Reached() {
		return fmt.Sprintf(
			"§5.6.4: %d of %d probes run this round against %s, so the cap is reached and "+
				"no further probe runs in this round; raise %s to run more",
			c.Count, c.Max, setting, setting)
	}
	return fmt.Sprintf("§5.6.4: %d of %d probes run this round against %s",
		c.Count, c.Max, setting)
}

// Err is §5.6.4's refusal: an error when the round has spent its budget, and
// nil while it has one left. It is asked before anything is done in the
// sandbox, so a refused run performs no experiment.
func (c RoundCap) Err() error {
	if c.Reached() {
		return &RoundCapReachedError{Cap: c}
	}
	return nil
}

// RoundCapReachedError reports a probe run refused because the round has
// already run probe.max_per_round probes, per §5.6.4.
//
// The cli layer maps it onto exit code 1, as it does §1.6.2's comment cap. The
// invocation is right and every file named was read; what refuses is the
// round's budget, and the only thing that changes it is the setting §2.7
// resolves.
type RoundCapReachedError struct {
	// Cap is the decision that refused the run, so the count and the cap
	// reach a caller as numbers and not only as text inside a message.
	Cap RoundCap
	// Performed marks a refusal met at the record's write rather than
	// before the run: the run was performed and its record is what is
	// refused, so the message must not say that nothing was run.
	Performed bool
}

func (e *RoundCapReachedError) Error() string {
	if e.Performed {
		return fmt.Sprintf(
			"%s: this run's record would be number %d, so it was not stored; raise %s, or open a "+
				"new round with cr brief when the head moves",
			e.Cap.Disclosure(), e.Cap.Count+1, setting)
	}
	return fmt.Sprintf(
		"%s: this run would be number %d, so nothing was run; raise %s, or open a new round "+
			"with cr brief when the head moves",
		e.Cap.Disclosure(), e.Cap.Count+1, setting)
}
