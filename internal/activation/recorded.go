package activation

import (
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
// §2.4.4's repository is answered here and not by a second caller. Activate
// requires a resolved profile, so a round opened where none matched has no axis
// decision to re-derive and what is still true is §4.5.3 alone — and two
// readers answering that case for themselves is two ways for `cr review` and
// `cr status` to disagree with `cr brief` about which axes looked.
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
	axes := Activation{
		Active:      []string{},
		Disabled:    []Disabled{},
		Unavailable: []intent.Unavailable{},
	}
	if unavailable, marked := recorded.Unavailability(); marked {
		axes.Unavailable = append(axes.Unavailable, unavailable)
	}
	return axes
}
