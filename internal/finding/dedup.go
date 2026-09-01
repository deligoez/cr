package finding

import (
	"cmp"
	"slices"

	"github.com/deligoez/cr/internal/git"
)

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

// gradeStrength is §6.2's three grades from strongest to weakest, which is what
// §6.4.2's "the highest grade" reads. severityStrength is §6.1's four the same
// way. Both repeat the order their constants are declared in, because a rank
// has to be a list somewhere and a list built by hand elsewhere is one a fourth
// value could be appended to the wrong end of.
var (
	gradeStrength    = []Grade{GradeProbed, GradeCited, GradeArgued}
	severityStrength = []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow}
)

// rankIn is a value's position in a strength order, and one past the end for a
// value that order does not list.
//
// Unlisted ranks last, for the reason role.Order puts an unresolved role id
// behind the whole corpus: §6.1.3 checks that a field is present and not that
// its value is one cr knows, so a severity or a grade cr does not recognise
// does reach here, and rank zero would hand it the representative slot of every
// group it appeared in. Last is the honest place for a value nothing can rank.
func rankIn[T comparable](order []T, value T) int {
	if at := slices.Index(order, value); at >= 0 {
		return at
	}
	return len(order)
}

// RepresentativeAt is the position in Records of the record §6.4.2 puts at the
// head of a duplicate group: the highest grade, then the highest severity, then
// the earliest role in corpus order per §2.5.5. Everything else the group holds
// is what §6.4.3 suppresses.
//
// It answers with a position rather than with the record, and the difference is
// not cosmetic in either direction. §6.4.3 needs the whole of "everything that
// is not the representative", which a position gives directly and a returned
// pointer only gives back by comparing identities. And a function handing back
// a *Finding is, in this package, how a record comes into being from bytes cr
// did not write — TestNoUnanchoredItemBecomesARecordByAnyDoor reads the
// package's own source for exactly that shape and requires every one of them to
// be driven through an unanchored item. This function opens no such door: every
// record it can name was already in the group the caller handed it.
//
// The order of the three comparisons is §6.4.2's and is not a preference. Grade
// outranks severity because severity is the agent's assertion about how much
// the defect matters while grade is cr's own measure of what the record rests
// on: §1.6 spends the reviewer's standing on the comment that gets posted, so
// the record with an experiment behind it speaks for the group ahead of the
// record that merely called itself critical.
//
// roleOrder is role.Order's comparison over role ids, taken as an argument
// rather than rebuilt here. §2.5.5's order is resolution layer first, the layer
// is nameable only inside internal/role, and that package's
// TestNothingOutsideThisPackageCanNameTheLayerARoleResolvedFrom holds it there
// — so this is the one shape the order can arrive in and there is no second
// definition of it to drift from.
//
// The three comparisons total-order a group only as far as the role: two
// records from one role, at one anchor, in one class, tied on grade and
// severity, compare equal. Arrival order settles that last tie, so the
// partition Groups built decides it and one input still has exactly one
// representative.
//
// A group is never empty: Groups creates one only around a record and only ever
// appends to it, so there is always a position to return.
func (g *Group) RepresentativeAt(roleOrder func(a, b string) int) int {
	best := 0
	for at, record := range g.Records {
		if outranks(record, g.Records[best], roleOrder) {
			best = at
		}
	}
	return best
}

// outranks reports whether record beats held under §6.4.2's three keys. It is
// strict, so an equal record never displaces the one already held and the
// earliest arrival wins a tie the three keys cannot break.
func outranks(record, held *Finding, roleOrder func(a, b string) int) bool {
	return cmp.Or(
		cmp.Compare(rankIn(gradeStrength, record.Grade), rankIn(gradeStrength, held.Grade)),
		cmp.Compare(rankIn(severityStrength, record.Severity), rankIn(severityStrength, held.Severity)),
		roleOrder(record.Role, held.Role),
	) < 0
}
