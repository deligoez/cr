package activation

import (
	"slices"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/profile"
)

// OfRound applies §4.5.1 to §4.5.3 to a round as `meta.json` recorded it: the
// profile the round resolved, the issue key §3.2 settled for it, and the
// `intent.key_pattern` §4.5.3's reason names.
//
// The inputs are what the round recorded rather than what the repository says
// now. §2.4's selection reads the repository's marker files and §3.2's
// resolution reads the pull request's branch, title, and body, and both can
// have moved since the round was opened; an axis decision drawn from them would
// be a decision about a round nobody opened.
//
// §2.4.4's repository is answered by Unprofiled, the one answer `cr brief` gives
// it too, so `cr review` and `cr status` cannot disagree with the brief about
// which axes looked.
//
// profileID is taken beside p because they answer different questions: p is the
// profile as it loads today, and profileID is whether the round resolved one at
// all. §2.4.4 loads the empty profile for a round that resolved none, and an
// empty profile is indistinguishable from one whose file has gone missing.
func OfRound(p *profile.Profile, profileID, issueKey, keyPattern string) Activation {
	recorded := intent.Recorded(issueKey, keyPattern)
	if profileID != "" {
		return Activate(p, recorded)
	}
	return Unprofiled(recorded)
}

// Unprofiled applies §4.5.1 to §4.5.3 to §2.4.4's repository, where no profile
// matched: "disable every axis that requires one, rather than guessing".
//
// Which axes require one is profile.Unmatched's list, read rather than restated,
// so the §2.4.4 report and the axes it describes are one answer. Every axis that
// list does not name still runs: the intent axis turns on an issue key, which
// §4.5.3 still asks for, and correctness and convention ask a profile for
// nothing — convention loses only its reinvention half, which §4.5.4 reports as
// a half rather than as an axis. Reporting those as not run would be the guess
// §2.4.4 forbids, aimed at the review instead of at the language.
func Unprofiled(i intent.Intent) Activation {
	missing := profile.Unmatched()
	ids := axis.IDs()
	a := Activation{
		Active:      make([]string, 0, len(ids)),
		Disabled:    make([]Disabled, 0, len(ids)),
		Unavailable: make([]intent.Unavailable, 0, len(ids)),
	}
	unavailable, marked := i.Unavailability()
	for _, id := range ids {
		switch {
		case id == axis.Intent && marked:
			a.Unavailable = append(a.Unavailable, unavailable)
		case slices.Contains(missing.Disabled, id):
			a.Disabled = append(a.Disabled, Disabled{
				Axis: id,
				Rule: RuleNoTestCommand,
				Reason: "no profile matched this repository, so no tests.cmd is declared for this axis to run; " +
					"set `profile` in the per-repository config to name the profile this repository is",
			})
		default:
			a.Active = append(a.Active, id)
		}
	}
	return a
}
