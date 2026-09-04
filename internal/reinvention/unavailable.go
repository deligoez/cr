package reinvention

import (
	"fmt"

	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/symbol"
)

// Unavailable is one entry of §4.5.4's report: a lens that did not run, and the
// reason it could not.
//
// §4.3.1 states the obligation directly — when no symbol index can be built,
// the reinvention half of this axis MUST be marked unavailable per §4.5 rather
// than skipped silently — and this is the shape that carries it. The lens is
// always profile.ReinventionLens, spelled there rather than here so §2.4.4's
// report about a repository no profile matched and this round's report about a
// profile that matched and declares no language name the same lens with the
// same string.
//
// It is deliberately not an axis. §4.3.5 sends the rest of the convention axis
// to the rule corpus of §2.6, whose per-repository and global layers outlive
// any profile, so the axis runs while this half of it does not. Reporting the
// axis out would claim more went unreviewed than did, and §4.5.4 is a report
// about what was not looked at, not a place to be safe in.
type Unavailable struct {
	// Lens is the lens that did not run.
	Lens string `json:"lens"`
	// Reason is why it could not, and what would make it run.
	Reason string `json:"reason"`
}

// Disclosure satisfies finding.HonestyDisclosure, so the entry reaches the
// reader through the one channel §11.1 exempts from `--quiet` rather than
// through a message a flag can silence. The text is derived from the two fields,
// so what is printed and what a caller reads as data cannot drift apart.
func (u Unavailable) Disclosure() string {
	return "lens " + u.Lens + " unavailable, per §4.3.1: " + u.Reason
}

// unavailability names why §4.3.1's reinvention half could not run, and reports
// false when it could.
//
// Three states reach it, and they are three different sentences to the person
// reading the report. A profile declaring no `symbols.lang` is a profile that
// never claimed a language — the shipped generic one, whose emptiness is what
// it has to say. A `symbols.lang` cr has no scanner for is the user having said
// which language this is and cr not being able to read it, which is a gap in cr
// and reads as one. An index that was asked for and did not arrive is neither,
// and naming it separately is what keeps a git read that failed from being
// reported as a profile that was misconfigured.
//
// Each names what would make the lens run, because §4.5.4's reason is read by
// someone deciding whether to act on it.
func unavailability(p *profile.Profile, index *symbol.Index) (Unavailable, bool) {
	switch {
	case p.Symbols.Lang == "":
		return reinventionOut(fmt.Sprintf(
			"profile %q declares no symbols.lang, so §4.3.1's symbol index cannot be built; "+
				"set symbols.lang in the profile to name this repository's language",
			p.ID,
		)), true
	case !symbol.Supported(p.Symbols.Lang):
		return reinventionOut(fmt.Sprintf(
			"profile %q declares symbols.lang %q, which cr has no symbol scanner for; "+
				"set symbols.lang to a language cr can index",
			p.ID, p.Symbols.Lang,
		)), true
	case index == nil:
		return reinventionOut(fmt.Sprintf(
			"cr built no symbol index for symbols.lang %q, so it has no pre-existing symbol to offer",
			p.Symbols.Lang,
		)), true
	}
	return Unavailable{}, false
}

// reinventionOut is one §4.5.4 entry for the lens of §4.3.1.
func reinventionOut(reason string) Unavailable {
	return Unavailable{Lens: profile.ReinventionLens, Reason: reason}
}
