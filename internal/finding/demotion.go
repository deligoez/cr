package finding

// DemotionBound is the sentence §7.3.5 requires cr to report the demotion rate
// as, and it is a constant so every renderer says it the same way.
//
// §7.3.5 is a rule about what the number may be presented as, not only about
// what it is computed from: marking a false positive `wrong` costs the reviewer
// an extra marker edit while deleting the block is free, so `discarded-wrong`
// is systematically under-counted and the rate can only understate imprecision.
// A reader who took it for a measured precision would demote a class on a
// number that was never measuring what they thought.
const DemotionBound = "§7.3.5: a lower bound on imprecision, not a measured precision — " +
	"marking a false positive `wrong` costs an extra edit while deleting the block is free, " +
	"so `discarded-wrong` is under-counted"

// Sample is §7.3.4's threshold and minimum, resolved from `stats` before a
// candidacy is decided.
//
// Both candidacies take the same pair, because §7.3.6 draws the volume
// candidacy "over the same sample": two knobs that could disagree would let a
// class be neither candidate at a rate at which it should have been one.
type Sample struct {
	// Threshold is `stats.demote_threshold`, which a rate must exceed.
	Threshold float64
	// MinSamples is `stats.min_samples`, the raises a class needs before
	// either rate is worth reading at all.
	MinSamples int
}

// DemotionRate is §7.3.4's rate for one class: `(discarded-wrong + softened) /
// raised`.
//
// `not-here` is absent from the numerator by construction rather than by a
// subtraction, because §7.3.4 excludes it for a reason that is not arithmetic:
// §7.2 makes it the ordinary case of a true finding not worth saying on this
// pull request, so it carries no evidence at all that the class is imprecise.
// Outcome.CountsAgainstClass is the one place that decision is spelled, and
// this reads it rather than restating it.
//
// A class with no raises has no rate. Zero is returned rather than a division
// by zero, and it is never a candidate anyway: §7.3.4 requires at least
// `stats.min_samples` raises, which is at least one.
func (c TriageCounts) DemotionRate() float64 {
	if c.Raised == 0 {
		return 0
	}
	counted := 0
	for _, action := range TriageActions() {
		outcome := Outcome(action)
		if action != ActionRaised && outcome.CountsAgainstClass() {
			counted += c.of(outcome)
		}
	}
	return float64(counted) / float64(c.Raised)
}

// of is one outcome's count, so a rate can be written in terms of §7.3.1's
// vocabulary rather than of this struct's field names.
func (c TriageCounts) of(outcome Outcome) int {
	switch outcome {
	case OutcomeKept:
		return c.Kept
	case OutcomeSoftened:
		return c.Softened
	case OutcomeDiscardedNotHere:
		return c.DiscardedNotHere
	case OutcomeDiscardedWrong:
		return c.DiscardedWrong
	default:
		return 0
	}
}

// DemotionCandidate is one class §7.3.4 lists, with the numbers that put it
// there.
//
// The rate is reported beside the raises rather than alone, because §7.3.7
// makes this a report to the user and a bare 0.75 is not something a user can
// act on: 3 of 4 and 30 of 40 are the same number and not the same evidence.
type DemotionCandidate struct {
	// Class is the class being proposed for demotion.
	Class string `json:"class"`
	// Raised is the sample the rate was taken over.
	Raised int `json:"raised"`
	// RateLowerBound is §7.3.4's rate, named for what §7.3.5 says it is.
	// The field carries the caveat so a reader who sees only the JSON
	// cannot mistake it for a measured precision.
	RateLowerBound float64 `json:"rate_lower_bound"`
}

// DemotionCandidates are the classes §7.3.4 lists: rate above the threshold,
// over at least the minimum raises.
//
// "Exceeds" is read strictly, so a class sitting exactly on the threshold is
// not a candidate. The section words both bounds and words them differently —
// "exceeds" the threshold, "at least" the minimum — and reading either the
// other way would move a class across the line on the default settings.
//
// §7.3.7 makes the result a report: nothing here changes a rule's kind, and
// there is no path from this value to a write.
func DemotionCandidates(classes []ClassTriage, over Sample) []DemotionCandidate {
	candidates := make([]DemotionCandidate, 0)
	for _, class := range classes {
		rate := class.DemotionRate()
		if class.Raised < over.MinSamples || rate <= over.Threshold {
			continue
		}
		candidates = append(candidates, DemotionCandidate{
			Class: class.Class, Raised: class.Raised, RateLowerBound: rate,
		})
	}
	return candidates
}
