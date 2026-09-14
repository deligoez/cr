package coverage

import (
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/activation"
	"github.com/deligoez/cr/internal/role"
)

// Skipped is §4.6.4's report: every role of §2.5.5's corpus the round left out,
// each carrying the reason it could not look.
//
// It is the negative of activation.ActiveRoles and it is derived rather than
// stored, because meta.json records the active set and §4.6.4 asks for the
// other one — "a role whose prerequisites are unmet MUST be reported as skipped
// with its reason, never omitted silently". A reader handed only the active set
// has to subtract two lists and then invent the reason, which is the silence
// that sentence forbids.
//
// It lives here, beside the SkippedRole it produces, because §4.6.4's report
// has two readers: `cr review` emits it with the fan-out, and `cr status`
// carries it in §10.1.3's lens list. Two derivations would be two answers to
// "did this role look", and the disagreement would be invisible — each command
// would be internally consistent and they would name different sets.
//
// active MUST be meta.json's recorded set rather than a recomputation. `cr
// brief` settles it once per round per §4.5.1, and a report deriving its own
// could call a role skipped that the round counted active — which §10.2.2 then
// demands a cell for.
//
// The order is the corpus's, so a role's line here and its cells elsewhere are
// read in the one order §2.5.5 fixes.
func Skipped(
	axes activation.Activation, corpus []role.Resolved, active []string, profileID string,
) []SkippedRole {
	out := make([]SkippedRole, 0, len(corpus))
	for i := range corpus {
		// Indexed rather than ranged by value: a Resolved carries a
		// whole Role, which is what gocritic's rangeValCopy is about.
		r := &corpus[i].Role
		if slices.Contains(active, r.ID) {
			continue
		}
		out = append(out, SkippedRole{
			Role: r.ID, Reason: skipReason(axes, r, profileID), author: skipAuthor(axes, r, profileID),
		})
	}
	return out
}

// skipReason says why one inactive role did not look.
//
// The axis is asked first because activation.ActiveRoles asks it first, so the
// reason a reader is given is the clause that actually decided. A role on an
// axis that did not run is out however its `profiles` list reads, and sending
// the reader to that list would send them to a fix that changes nothing.
//
// Where the axis did not run, the axis's own entry is reused verbatim rather
// than reworded, so a reader acting on the role's line and one acting on the
// axis's line are sent to the same fix.
//
// Where the axis did run, activation.ActiveRoles is asked about the role alone
// rather than its `profiles` clause being restated here. A list that excludes
// the resolved profile is what decided, and the sentence names that list. A
// list that admits it — an empty one, which admits every profile, or one naming
// it — decided nothing: the role would be counted active today, so what left it
// out is the active-role set `cr brief` recorded for the round, and sending the
// reader to the list would send them to a fix that changes nothing.
func skipReason(axes activation.Activation, r *role.Role, profileID string) string {
	if slices.Contains(axes.Active, r.Axis) {
		if len(axes.ActiveRoles([]role.Resolved{{Role: *r}}, profileID)) != 0 {
			return notRecordedReason(r, profileID)
		}
		return "its profiles list names " + strings.Join(r.Profiles, ", ") +
			" and this round resolved profile " + namedProfile(profileID) +
			"; the role looks only under a profile it names"
	}
	for _, entry := range axes.Disclosures() {
		if strings.HasPrefix(entry.Disclosure(), "axis "+r.Axis+" ") {
			return entry.Disclosure()
		}
	}
	return "its axis " + r.Axis + " did not run this round"
}

// skipAuthor is skipReason's decision worded for the pull request's author:
// the same clause decides, and the author is told what it means for this change
// rather than what would change it.
func skipAuthor(axes activation.Activation, r *role.Role, profileID string) string {
	said := "role " + r.ID + " did not look at this change: "
	switch {
	case !slices.Contains(axes.Active, r.Axis):
		return said + "axis " + r.Axis + " did not run"
	case len(axes.ActiveRoles([]role.Resolved{{Role: *r}}, profileID)) != 0:
		return said + "it was not among the reviewers of this round"
	}
	return said + "it reviews only other kinds of repository than this one"
}

// notRecordedReason is the reason for a role the definition admits and the
// round did not count: its axis ran and its `profiles` list admits the resolved
// profile, and meta.json's active roles, which `cr brief` settles per §4.5.1,
// do not name it. That happens when the corpus or the profile changed after the
// brief, and a brief run again at the same head records the set anew.
//
// The list is described rather than joined, because an empty list joined is an
// empty name, and a sentence naming nothing reads as a list naming a profile.
func notRecordedReason(r *role.Role, profileID string) string {
	admits := "is empty, which admits every profile"
	if len(r.Profiles) != 0 {
		admits = "names profile " + profileID
	}
	return "this round's active roles, which `cr brief` recorded per §4.5.1, do not name it, " +
		"though its axis " + r.Axis + " ran and its profiles list " + admits +
		"; run `cr brief` again to record the round's active roles anew"
}

// namedProfile names the profile the round resolved, and says plainly when
// there was none: §2.4.4's repository matched no profile, and an empty string
// in the sentence would read as a profile whose id is empty.
func namedProfile(profileID string) string {
	if profileID == "" {
		return "none, per §2.4.4"
	}
	return profileID
}
