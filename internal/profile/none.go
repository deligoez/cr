package profile

import (
	"strings"

	"github.com/deligoez/cr/internal/axis"
)

// ReinventionLens names the half of the convention axis §4.3.1 builds out of a
// symbol index. It is deliberately not an axis id: reporting it as one would
// claim the whole of §4.3 went unreviewed when only the reinvention search did,
// because §4.3.5 sends the rest of that axis to the rule corpus of §2.6, whose
// per-repository and global layers outlive any profile.
const ReinventionLens = axis.Convention + "/reinvention"

// MissingProfile is the report §2.4.4 requires of a repository no profile
// matched: the situation named, and every axis that needs a profile switched
// off, rather than a language guessed from whatever cr can see.
//
// Which lenses those are is settled in three places rather than one, and two of
// the three do not answer to the profile at all.
//
//   - §4.5.2 disables the test axis when the profile declares no `tests.cmd`.
//     A repository with no profile has no `tests.cmd` to declare, so the test
//     axis is disabled.
//   - §4.3.1 needs a symbol index built from `symbols.lang`, so the reinvention
//     half of the convention axis is unavailable. The axis itself stands: only
//     the profile's own third layer of §2.6 item 1 is gone.
//   - §4.5.3 marks the intent axis unavailable when no issue key resolves,
//     which is a tracker question rather than a profile one, and the
//     correctness axis of §4.2 reads claims and units and asks a profile for
//     nothing. Neither is taken out here.
//
// Reporting intent or correctness as not run would be §2.4.4's own guess
// pointed the other way — a lens claimed dead that could have looked — and
// §4.5.4 is a report about lenses that did not run, not a place to be safe in.
//
// `disabled` and `unavailable` are §4.5's two words for two different things
// and this type keeps them apart: a disabled lens was switched off by what the
// configuration declares, an unavailable one was left without a prerequisite it
// cannot supply itself.
type MissingProfile struct {
	// Disabled names the axis ids of §1.5 that a missing profile switches
	// off, every one of them for the reason §4.5.2 gives.
	Disabled []string `json:"disabled"`
	// Unavailable names the lenses smaller than a whole axis that a missing
	// profile leaves without a prerequisite, every one of them for the
	// reason §4.3.1 gives.
	Unavailable []string `json:"unavailable"`
}

// Missing returns the §2.4.4 report for the third state of Selection, and false
// for the other two: a profile was selected, or profiles tied and Err already
// carries the abort §2.4.2 requires.
//
// It is the empty state's counterpart to Err, and the shape differs because the
// outcomes do. A tie stops the command; nothing matching does not, so what
// comes back is a report to print and a set of lenses to leave out, never an
// error. Nothing here picks a profile: §2.4.3 keeps `generic` out of automatic
// selection, so falling back to it would be exactly the guess §2.4.4 forbids,
// and naming it stays the user's through the per-repository config.
func (s *Selection) Missing() (MissingProfile, bool) {
	if s.Selected || len(s.Tied) > 0 {
		return MissingProfile{Disabled: []string{}, Unavailable: []string{}}, false
	}
	return MissingProfile{
		Disabled:    []string{axis.Test},
		Unavailable: []string{ReinventionLens},
	}, true
}

// Disclosure is the §11.1 honesty disclosure of §2.4.4, and satisfies the
// finding.HonestyDisclosure contract the writer that holds the `--quiet`
// exemption will consume, so the report reaches the reader through the one
// channel §11.1 exempts rather than through a message a flag can silence.
//
// A missing profile silently narrowing the review is the failure §2.4.4 and
// §4.5.4 both name: the round would come back complete with two lenses that
// never looked, and the author would read the silence as a clean result.
//
// The text is derived from the two fields, so what is printed and what a caller
// reads as data cannot disagree. One reason per category is honest because a
// missing profile is one cause: an entry that is out for some other reason
// belongs in some other report.
func (m MissingProfile) Disclosure() string {
	// The `+` survives mutation, and is equivalent for a narrower reason
	// than a capacity hint: a negative capacity panics, and Missing above
	// is the only constructor, filling the two fields one for one. The
	// concatenation in ReinventionLens above is reported NOT COVERED for
	// an unrelated reason — it is a constant string expression, and the
	// mutated form does not compile.
	lenses := make([]string, 0, len(m.Disabled)+len(m.Unavailable))
	for _, id := range m.Disabled {
		lenses = append(lenses, "axis "+id+" disabled, per §4.5.2")
	}
	for _, name := range m.Unavailable {
		lenses = append(lenses, "lens "+name+" unavailable, per §4.3.1")
	}
	return "no profile matched this repository, so cr disabled every axis that needs one rather than guessing: " +
		strings.Join(lenses, "; ") +
		"; set `profile` in the per-repository config to name the profile this repository is"
}
