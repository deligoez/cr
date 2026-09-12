package coverage

import "github.com/deligoez/cr/internal/finding"

// SkippedRole is §4.6.4's entry in §4.5.4's report: a role whose prerequisites
// were unmet, named together with the reason it could not look.
//
// It is a fourth shape rather than a reuse of the three that exist, because a
// role is not an axis and not a lens. activation.Disabled reports an axis the
// configuration switched off, intent.Unavailable an axis left without a
// prerequisite, and reinvention.Unavailable and testadequacy.Unavailable the
// halves of §4.3.1 and §4.4.1. A role skipped per §4.6.4 is none of those: its
// axis ran, the lens ran, and one reviewer of §2.5 did not — and §4.5.4 lists
// it separately for the reason it lists the other three separately, that the
// four are four different sentences to whoever reads the report.
//
// The reason is carried rather than derived, per §4.5.4 and §12.4: a reader
// deciding whether to act on a skipped role needs what would make it run.
type SkippedRole struct {
	// Role is the role id of §2.5 that did not look.
	Role string `json:"role"`
	// Reason is why its prerequisites were unmet, and what would meet them.
	Reason string `json:"reason"`
}

// Disclosure satisfies finding.HonestyDisclosure, so the entry reaches the
// reader through the one channel §11.1 exempts from `--quiet` rather than
// through a message a flag can silence. The text is derived from the two
// fields, so what is printed and what a caller reads as data cannot drift.
func (s SkippedRole) Disclosure() string {
	return "role " + s.Role + " skipped, per §4.6.4: " + s.Reason
}

// Lenses is §4.5.4's report for one round: every lens that did not run, with
// its reason.
//
// §4.5.4 names four kinds and they are produced in three different places —
// internal/activation settles the axes, internal/reinvention and
// internal/testadequacy the halves of §4.3.1 and §4.4.1, and §4.6.4 the roles.
// Collecting them here rather than at each printing site is what makes the
// obligation checkable: a caller holds one value, and a kind added to it
// reaches every reader at once instead of reaching whichever call site somebody
// remembered.
//
// Every field is a slice of entries that did not run. An empty Lenses is a
// round where every lens looked, which is a report saying so rather than the
// absence of one.
type Lenses struct {
	// Axes holds §4.5.4's first two kinds, as internal/activation settles
	// them: the disabled axes and the unavailable ones, in §1.5 order.
	Axes []finding.HonestyDisclosure
	// Halves holds the lens halves that could not run — §4.3.1's
	// reinvention half and §4.4.1's symbol half — each already carrying
	// the section it is reported under.
	Halves []finding.HonestyDisclosure
	// Roles holds §4.6.4's skipped roles.
	Roles []SkippedRole
}

// Disclosures collects the four kinds in the order §4.5.4 names them: the
// disabled and unavailable axes, then the unavailable halves, then the skipped
// roles.
//
// The result is never nil, per the §12.3 convention, so a round with nothing to
// report serialises `[]` rather than `null`.
func (l Lenses) Disclosures() []finding.HonestyDisclosure {
	// The capacity is a sum of lengths, never a difference: `make` panics
	// on a negative capacity, and len is non-negative for every slice, the
	// nil one included, so this sum cannot reach one.
	out := make([]finding.HonestyDisclosure, 0, len(l.Axes)+len(l.Halves)+len(l.Roles))
	out = append(out, l.Axes...)
	out = append(out, l.Halves...)
	for _, skipped := range l.Roles {
		out = append(out, skipped)
	}
	return out
}

// Verdict is §10.2's completeness answer as it is printed: the verdict, the
// exact reason when it is false, and every lens of §4.5.4 that did not run.
//
// It is a method on the report rather than a function beside it, and that is
// the whole of what this type enforces. §10.2 requires a verdict of complete to
// be printed together with every lens that did not run, because completeness
// across three axes is not completeness across four and neither is completeness
// with a role that never looked. A renderer that took only the verdict could
// print one without the list and would read as a stronger claim than the round
// supports; here there is no way to reach the sentence without holding the
// report, so the two are one call.
//
// The wording states what cr observed and nothing further: §10.2 forbids cr to
// approve the pull request or to describe a complete round as a settled review,
// and whether the author addressed anything is outside what cr can see.
func (l Lenses) Verdict(complete bool, reason string) []string {
	lines := make([]string, 0, 1+len(l.Axes)+len(l.Halves)+len(l.Roles))
	lines = append(lines, verdictLine(complete, reason))
	for _, entry := range l.Disclosures() {
		lines = append(lines, entry.Disclosure())
	}
	return lines
}

// verdictLine words §10.2's verdict. A false verdict carries the exact reason,
// and a true one carries none: there is no reason a round is complete beyond
// the four conditions all holding, and inventing one would be cr forming a
// judgement about a round it only counted.
func verdictLine(complete bool, reason string) string {
	if complete {
		return "round complete, per §10.2"
	}
	return "round not complete, per §10.2: " + reason
}
