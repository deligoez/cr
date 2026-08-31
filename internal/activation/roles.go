package activation

import (
	"slices"

	"github.com/deligoez/cr/internal/role"
)

// ActiveRoles applies the role half of §4.5.1 to one round's corpus: a role is
// active when its axis is active and its `profiles` list is empty or names the
// resolved profile.
//
// That is the whole definition, and the omission is the point. §4.5.1 also
// names §4.6.4 — "and §4.6.4 did not report it skipped" — and reading that as a
// third input makes the definition circular: §4.6.4 is a report `cr review`
// emits, `cr review` fans out over the active roles, so activeness would be
// defined in terms of an output of the command that consumes it. §4.5.6 rejects
// a cell naming an inactive role and §10.2.2 demands a complete row of cells for
// every active role, and neither may wait on a command that may never have run.
// So §4.6.4 is read as the reporting obligation for the negative case: a role
// this function leaves out is a role `cr review` must report skipped with its
// reason rather than omit, which is what §4.5.4 already requires of it.
//
// corpus arrives in §2.5.5's order, which role.Resolve produced, and the result
// keeps it: the active set is a subset of the corpus rather than a set of its
// own, so nothing here can reorder what §6.4.2 reads as corpus order.
//
// profileID is the resolved profile of §2.4, empty for §2.4.4's repository
// where none matched. An empty id matches no `profiles` entry, which is the
// answer that falls out rather than one written here: a role scoped to a
// profile cannot be active where there is no profile to scope it to, and every
// axis is off in that state anyway.
func (a Activation) ActiveRoles(corpus []role.Resolved, profileID string) []string {
	active := make([]string, 0, len(corpus))
	for i := range corpus {
		// Indexed rather than ranged by value: a Resolved carries a
		// whole Role, and copying one per iteration is what gocritic's
		// rangeValCopy is about.
		r := &corpus[i].Role
		if !slices.Contains(a.Active, r.Axis) {
			continue
		}
		if len(r.Profiles) != 0 && !slices.Contains(r.Profiles, profileID) {
			continue
		}
		active = append(active, r.ID)
	}
	return active
}
