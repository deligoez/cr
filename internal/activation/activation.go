// Package activation answers §4.5's first three items for one round: which axes
// of §1.5 run, and — for every one that does not — why.
//
// The three rules are not one rule with three inputs, and the report is why.
// §4.5.4 requires every lens that did not run to appear with its reason, §3.7.6
// asks the brief for "the active, disabled, and unavailable axes with their
// reasons", and §10.1.3 asks `cr status` for the same three. That is three
// categories, not a boolean. "disabled by configuration" and "unavailable: no
// issue key resolved" are different sentences to a reader, and a design that
// collapses them keeps the count and loses the one thing the honesty obligation
// exists to carry — what was not looked at, and what would make it look.
//
// So the reason is carried rather than re-derived. An axis leaves here as one of
// Active, as a Disabled naming the item of §4.5 that switched it off, or as the
// intent.Unavailable §4.5.3 already defines. Nothing here declares a second
// unavailability shape: intent.Unavailable, testadequacy.Unavailable and
// profile.MissingProfile all satisfy finding.HonestyDisclosure, and that
// interface is already §4.5.4's union. A fourth struct with the same two fields
// would split the truth about an unavailable intent axis across two types that
// can disagree.
//
// Two ordering decisions are load-bearing.
//
// Prerequisites are checked before the configuration. §4.5.2 and §4.5.3 are
// written without a condition — the test axis MUST be disabled when the profile
// declares no `tests.cmd`, the intent axis MUST be marked unavailable when no
// issue key resolves — while §4.5.1 makes activity the conjunction of enabled
// and met. Checking the configuration first would let a profile that switches an
// axis off suppress a prerequisite the spec states with no exception. The
// consequence worth stating is the one it gives a reader: a Disabled citing
// §4.5.1 always means the configuration alone kept the axis out.
//
// The profile's `axes` object is "the resolved configuration" of §4.5.1. The
// closed key set in internal/config names no axis and drops any key it does not
// carry, so §2.4's `axes` is the only layer that states an enabled state at all,
// and the §2.7 stack reaches this decision through which profile it resolves
// rather than through a setting of its own.
package activation

import (
	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/profile"
)

// The items of §4.5 that switch an axis off. A Disabled carries one of them so
// the two disables stay two: one is the user's declaration, the other is a
// prerequisite the axis cannot supply itself, and §4.5.4 reports a reason rather
// than a state.
const (
	// RuleConfigured is §4.5.1: the resolved configuration does not enable
	// the axis.
	RuleConfigured = "§4.5.1"
	// RuleNoTestCommand is §4.5.2: the profile declares no `tests.cmd`, so
	// the test axis is disabled automatically.
	RuleNoTestCommand = "§4.5.2"
)

// Disabled is one entry of §4.5.4's report for an axis that was switched off:
// the axis, the item of §4.5 that switched it off, and the reason in the words a
// reader can act on.
//
// It is deliberately not intent.Unavailable under another name. §4.5.4 lists "a
// disabled axis, an unavailable axis" as two of its four kinds, and §4.5.3 says
// "marked unavailable" where §4.5.2 says "disabled": a disabled axis was
// switched off by what the configuration declares, an unavailable one was left
// without a prerequisite it cannot supply itself. Reporting the first as the
// second would tell the author cr was prevented from looking when it was told
// not to.
type Disabled struct {
	// Axis is the axis id of §1.5 that did not run.
	Axis string `json:"axis"`
	// Rule is the item of §4.5 that switched it off, RuleConfigured or
	// RuleNoTestCommand.
	Rule string `json:"rule"`
	// Reason is why it is off, and what would turn it on.
	Reason string `json:"reason"`
}

// Disclosure satisfies finding.HonestyDisclosure, so the entry reaches the
// reader through the one channel §11.1 exempts from `--quiet` rather than
// through a message a flag can silence. The text is derived from the three
// fields, so what is printed and what a caller reads as data cannot drift apart.
func (d Disabled) Disclosure() string {
	return "axis " + d.Axis + " disabled, per " + d.Rule + ": " + d.Reason
}

// Activation is one round's answer to §4.5.1: the axes that run, and the two
// kinds of entry §4.5.4 owes the reader for the ones that do not.
//
// Every slice is empty rather than nil, per the §12 convention, so a round with
// nothing disabled serialises `[]` and not `null`.
type Activation struct {
	// Active holds the axis ids of §1.5 that run this round, in §1.5 order.
	Active []string `json:"active"`
	// Disabled holds the §4.5.4 entries for the axes the configuration or
	// §4.5.2 switched off, in §1.5 order.
	Disabled []Disabled `json:"disabled"`
	// Unavailable holds the §4.5.4 entries for the axes a missing
	// prerequisite left unavailable. §4.5.3 is the only one v0.1 states, so
	// this is intent.Unavailable and never a shape of this package's own.
	Unavailable []intent.Unavailable `json:"unavailable"`
}

// Disclosures collects every axis that did not run as the disclosure contract
// §11.1 exempts from `--quiet`, so a caller printing §4.5.4's list, §10.1.3's
// report, or §8.3.3's review body hands the writer one slice and cannot forget a
// category.
//
// Disabled entries come first, then unavailable ones, each in §1.5 order,
// because §3.7.6 and §10.1.3 both name the categories in that order. The result
// is empty when every axis ran, which is a report saying so rather than the
// absence of one.
func (a Activation) Disclosures() []finding.HonestyDisclosure {
	// gremlins reports the `+` here as a surviving ARITHMETIC_BASE mutant,
	// the same equivalent profile.MissingProfile.Disclosure already carries:
	// the sum is a capacity hint, and a wrong one changes how often append
	// reallocates and nothing a test can observe. The three NOT COVERED
	// entries gremlins reports against the bare `switch` in Activate are the
	// known attribution artifact on case clauses; this package is at 100%
	// statement coverage.
	out := make([]finding.HonestyDisclosure, 0, len(a.Disabled)+len(a.Unavailable))
	for _, d := range a.Disabled {
		out = append(out, d)
	}
	for _, u := range a.Unavailable {
		out = append(out, u)
	}
	return out
}

// Activate applies §4.5.1 to §4.5.3 over the four axes of §1.5.
//
// p is the resolved profile of §2.4 and must not be nil. The repository no
// profile matched is §2.4.4's state, and profile.Selection.Missing already
// reports it — deriving a second answer here would give one run two reports that
// can disagree about which axes looked.
//
// i is the round's §3.2 resolution. Whether the intent axis is unavailable is
// asked of it rather than recomputed, so there is no Intent carrying issue text
// whose axis is reported out, and none resolving no key whose axis is reported
// in.
func Activate(p *profile.Profile, i intent.Intent) Activation {
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
		case id == axis.Test && len(p.Tests.Cmd) == 0:
			a.Disabled = append(a.Disabled, Disabled{
				Axis: id,
				Rule: RuleNoTestCommand,
				Reason: "the resolved profile " + p.ID +
					" declares no tests.cmd, so this axis has no runner to reach; add tests.cmd and tests.globs to the profile",
			})
		case !p.Axes[id]:
			a.Disabled = append(a.Disabled, Disabled{
				Axis: id,
				Rule: RuleConfigured,
				Reason: "the resolved profile " + p.ID +
					" does not enable axes." + id + "; set it to true to run this axis",
			})
		default:
			a.Active = append(a.Active, id)
		}
	}
	return a
}
