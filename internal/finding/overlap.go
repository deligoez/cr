package finding

import (
	"fmt"
	"slices"
)

// Overlap is §6.4.3's overlap summary for one duplicate group: which roles saw
// the same defect at the same anchored line, and which of their records speaks
// for them.
//
// It exists because a suppressed duplicate is otherwise invisible. §6.4.3 keeps
// the record in state `duplicate` rather than dropping it, so the round's state
// is complete either way; what the summary adds is the reading — three roles
// converging on one line is a different fact about the change from one role
// noticing it, and §1.6 has the reviewer spend their standing on the second as
// heavily as on the first.
//
// It carries no record beyond the representative's id. The suppressed records
// say for themselves which group they were retired into, through the
// `duplicate_of` §6.4.3 writes onto each of them, and a second copy of that
// list here would be a second thing to keep in agreement with them.
type Overlap struct {
	// Key is §6.4.1's key the group formed around, so a reader can see the
	// file, the side, the line and the class the roles converged on.
	Key DedupKey `json:"key"`
	// Representative is the id of the record §6.4.2 chose, the one that
	// reaches the draft.
	Representative string `json:"representative"`
	// Roles are the contributing role ids, each named once, in the order
	// their first record arrived. §6.4.3 asks for the roles and not for a
	// record count, and a role that filed two records in one group
	// contributed one opinion twice rather than two roles' worth.
	Roles []string `json:"roles"`
	// Suppressed is how many of the group's records §6.4.3 retired, which
	// is every record but the representative.
	Suppressed int `json:"suppressed"`
}

// Overlaps is the overlap summary of a whole round: one entry per duplicate
// group, in the order Groups produced those groups, and nothing for the groups
// that held a single record.
//
// A group of one is left out on purpose. Every finding is in some group, so a
// summary listing all of them would be the round's findings under another name;
// what §6.4.3 asks to be reported is where roles overlapped.
type Overlaps []Overlap

// Suppressed is how many records §6.4.3 retired across the whole round.
func (o Overlaps) Suppressed() int {
	total := 0
	for _, overlap := range o {
		total += overlap.Suppressed
	}
	return total
}

// Disclosure is §10.1.6's duplicate count, one of the seven reports §11.1
// exempts from `--quiet`, in the shape cap.go's HonestyDisclosure fixes.
//
// It is printed at zero as well. A count that appeared only when it was
// non-zero would leave a reader unable to tell a round in which no two roles
// overlapped from a round in which the dedup step was never reached, and the
// second is the one worth knowing about: §6.4.1 groups on the anchored line, so
// a merge that silently grouped nothing still produces a draft that looks
// complete.
func (o Overlaps) Disclosure() string {
	return fmt.Sprintf("%d records suppressed as duplicates across %d anchored lines",
		o.Suppressed(), len(o))
}

// MarkDuplicates applies §6.4.2 and §6.4.3 over one round's records and returns
// the overlap summary.
//
// It groups the records per §6.4.1, picks each group's representative per
// §6.4.2, and writes that representative's id into every other record's
// `duplicate_of`. It writes nothing else: §6.5.1 makes `duplicate_of` the one
// computed field `cr merge` may put in its output and leaves `state` to
// `cr record`, so the state §6.4.3 names is stamped where §9.1's table has an
// actor for it and not here.
//
// The records are edited in place rather than copied, because the caller holds
// the same pointers and is about to write them out. Nothing is reordered, so
// the merged file is still in the order the role files were read.
//
// roleOrder is role.Order's comparison, for the reason RepresentativeAt gives.
func MarkDuplicates(records []*Finding, roleOrder func(a, b string) int) Overlaps {
	overlaps := make(Overlaps, 0)
	for _, group := range Groups(records) {
		if len(group.Records) == 1 {
			continue
		}
		at := group.RepresentativeAt(roleOrder)
		representative := group.Records[at]
		roles := make([]string, 0, len(group.Records))
		for i, record := range group.Records {
			if i != at {
				record.DuplicateOf = representative.ID
			}
			if !slices.Contains(roles, record.Role) {
				roles = append(roles, record.Role)
			}
		}
		overlaps = append(overlaps, Overlap{
			Key:            group.Key,
			Representative: representative.ID,
			Roles:          roles,
			Suppressed:     len(group.Records) - 1,
		})
	}
	return overlaps
}
