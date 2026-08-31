package finding

import "github.com/deligoez/cr/internal/git"

// DedupKey is §6.4.1's dedup key: the anchored line and the defect class, so
// two roles that saw one defect in one place produce one comment rather than
// two. §1.6 is why it matters more than a count — a duplicate spends the
// reviewer's standing twice for one observation.
//
// §6.4.1 writes the key as `(anchor.path, anchor.line, class)` and `anchor.side`
// joins it, per round 8's dedup-key-omits-side and side-omitted-from-identity-keys
// and round 12's dedup-key-drops-side. §9.2 makes the side part of an anchor and
// §6.1.2 resolves the two sides against different trees, so a record about a
// removed line at merge-base line 40 and a record about an added line at head
// line 40 of the same file, in the same class, are not about the same code —
// they are not even about the same file version. Keyed without the side they are
// one group, and §6.4.3 then retires one of them in state `duplicate` for a
// reason no reader could observe.
//
// The line is `anchor.line` and not `anchor.start_line`, which is §6.4.1's own
// choice of field: a multi-line anchor keys on its last line.
//
// It is not WaiverKey, which carries the anchor's content hash where this
// carries the line number. Dedup groups records inside one round against one
// head, where a line number names code; a waiver outlives the round and has to
// follow the code as the file above it moves, which is what §7.4.2's "the same
// unchanged code" means.
type DedupKey struct {
	// Path is the anchored file, from `anchor.path`.
	Path string `json:"path"`
	// Side is the tree the line is counted in, from `anchor.side`: RIGHT
	// in the head, LEFT in the merge base (§9.2.1).
	Side git.Side `json:"side"`
	// Line is the last line of the anchored range, from `anchor.line`.
	Line int `json:"line"`
	// Class is the record's defect class, kebab-case per §6.1 and held to
	// that form by ValidateClass — two spellings of one class would be two
	// keys, and the duplicate would go unsuppressed.
	Class string `json:"class"`
}

// DedupKeyOf reads one record's dedup key.
//
// It is the only construction of that key, for the reason WaiverKeyOf is the
// only construction of §7.4.1's: the grouping §6.4.1 does, the representative
// §6.4.2 picks out of a group, and the overlap summary §6.4.3 reports all read
// the same four fields off the same record. A second spelling somewhere else
// would drift the day a fifth field joined one of them.
//
// The record is taken by pointer because it is a wide struct and nothing here
// writes to it.
func DedupKeyOf(record *Finding) DedupKey {
	return DedupKey{
		Path:  record.Anchor.Path,
		Side:  record.Anchor.Side,
		Line:  record.Anchor.Line,
		Class: record.Class,
	}
}

// Group is one duplicate group of §6.4: every record sharing one dedup key.
//
// A group of one is a group. §6.4.2 picks a representative out of every group
// and §6.4.3 suppresses whatever the representative is not, so a record nobody
// duplicated is the representative of its own group and nothing is suppressed —
// which is the same rule applied, not a case exempt from it.
type Group struct {
	// Key is the dedup key every record in the group carries.
	Key DedupKey `json:"key"`
	// Records are the records that carry it, in the order they arrived.
	Records []*Finding `json:"records"`
}

// Groups partitions records by §6.4.1's key.
//
// The groups come back in the order their keys first appear, and each group
// holds its records in arrival order, so the partition of one input is one
// partition however the map underneath it happens to iterate. §6.4.2's
// representative is a total order over a group and does not depend on this,
// but the overlap summary §6.4.3 reports does: contributing roles listed in a
// different order on every run would make one round's summary unequal to
// itself.
func Groups(records []*Finding) []Group {
	at := make(map[DedupKey]int, len(records))
	groups := make([]Group, 0, len(records))
	for _, record := range records {
		key := DedupKeyOf(record)
		if i, seen := at[key]; seen {
			groups[i].Records = append(groups[i].Records, record)
			continue
		}
		at[key] = len(groups)
		groups = append(groups, Group{Key: key, Records: []*Finding{record}})
	}
	return groups
}
