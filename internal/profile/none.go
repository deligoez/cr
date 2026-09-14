package profile

import "github.com/deligoez/cr/internal/axis"

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
	return Unmatched(), true
}

// Unmatched is §2.4.4's report on its own, for a reader that holds no Selection:
// a round whose meta.json recorded no profile is the same repository Missing
// reports on, and answering it from a second list would let `cr review` and
// `cr status` name different lenses than `cr brief` did.
func Unmatched() MissingProfile {
	return MissingProfile{
		Disabled:    []string{axis.Test},
		Unavailable: []string{ReinventionLens},
	}
}
