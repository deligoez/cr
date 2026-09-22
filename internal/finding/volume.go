package finding

// NotHereRate is §7.3.6's rate for one class: the share of its raises the
// reviewer discarded or withdrew as `not-here`. Outcome.NotHere is the one place
// that set is spelled, and this reads it rather than restating it.
//
// It is the complement of nothing. §7.3.4's rate and this one are computed over
// the same denominator and share no outcome at all, because the two dispositions
// of §7.2 mean opposite things: `wrong` says the finding was false and `not-here`
// says it was true and not worth a comment here. A class can therefore score
// high on one, high on the other, or high on neither, and the remedies differ —
// which is why §7.3.6 asks for a separate list rather than a second column.
func (c TriageCounts) NotHereRate() float64 {
	if c.Raised == 0 {
		return 0
	}
	counted := 0
	for _, action := range TriageActions() {
		outcome := Outcome(action)
		if action != ActionRaised && outcome.NotHere() {
			counted += c.of(outcome)
		}
	}
	return float64(counted) / float64(c.Raised)
}

// VolumeCandidate is one class §7.3.6 lists: accurate, and rarely worth
// posting.
//
// It carries no lower-bound caveat, and that is the difference from a demotion
// candidate rather than an omission. §7.3.5's under-counting is about the
// `wrong` marker costing an edit that plain deletion does not; `not-here` is
// what cr itself writes when a block is deleted, so this rate is measured on
// the action the reviewer takes for free.
type VolumeCandidate struct {
	// Class is the class being proposed for silence.
	Class string `json:"class"`
	// Raised is the sample the rate was taken over.
	Raised int `json:"raised"`
	// NotHereRate is the share of those raises discarded as `not-here`.
	NotHereRate float64 `json:"not_here_rate"`
	// Remedy is §7.3.6's own sentence about what to do, carried on the
	// candidate because the whole point of the separate list is that the
	// action differs: a class listed here is not imprecise, so softening
	// it would turn a true finding nobody wanted into a question nobody
	// wanted.
	Remedy string `json:"remedy"`
}

// VolumeRemedy is §7.3.6's sentence, as a constant so every candidate carries
// the same one.
const VolumeRemedy = "§7.3.6: accurate but rarely worth posting — " +
	"the remedy is to stop raising it, not to soften it"

// VolumeCandidates are the classes §7.3.6 lists: `not-here` rate above the
// threshold, over the same sample §7.3.4 uses.
//
// "The same sample" is read as the same two settings rather than as the same
// number arrived at separately, so a project that moves either knob moves both
// candidacies together — a class that fell off one list and onto neither would
// be a class cr had stopped having an opinion about for no stated reason.
//
// §7.3.7 makes the result a report and nothing more. There is no writer here
// and no caller that turns a candidate into a change: a rule's `kind` is
// altered by a person editing a rule file, and this package offers no way to
// alter one at all.
func VolumeCandidates(classes []ClassTriage, over Sample) []VolumeCandidate {
	candidates := make([]VolumeCandidate, 0)
	for _, class := range classes {
		rate := class.NotHereRate()
		if class.Raised < over.MinSamples || rate <= over.Threshold {
			continue
		}
		candidates = append(candidates, VolumeCandidate{
			Class: class.Class, Raised: class.Raised,
			NotHereRate: rate, Remedy: VolumeRemedy,
		})
	}
	return candidates
}
