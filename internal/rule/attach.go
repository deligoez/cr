package rule

import (
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/unit"
)

// Attachment is §4.3.6's answer for one unit: the mechanical rule hits that
// unit contains.
//
// There is an entry for every unit, carrying an empty list for a unit no rule
// matched. §4.6.1 emits one prompt per role and unit, and a unit missing from
// this answer would be a prompt with nothing said about rules at all — which a
// role reads as the same thing as a unit cr checked and found clean. §12.3's
// empty array says the second, and only the second.
type Attachment struct {
	// Unit is the §3.4.6 unit id.
	Unit string `json:"unit"`
	// Hits are the hits inside it, in the order §2.6.1.1's evaluation
	// produced them: corpus order, then diff order.
	Hits []Hit `json:"hits"`
}

// Reviewed keeps the hunks of the files that formed one of units, in the order
// given, and is the diff §2.6.1.1's evaluation reads.
//
// A file §3.4.2 excluded or §3.4.7 listed forms no unit, so no role reviews it.
// A hit in such a file would still reach `rule-stats.ndjson` under §2.6.1.6, and
// §6.2.5 would then stamp `origin: rule` on a citation into a file cr declined
// to review.
func Reviewed(hunks []git.Hunk, units []unit.Unit) []git.Hunk {
	formed := make(map[string]bool, len(units))
	for at := range units {
		formed[units[at].Path] = true
	}
	kept := make([]git.Hunk, 0, len(hunks))
	for at := range hunks {
		if formed[hunks[at].Path] {
			kept = append(kept, hunks[at])
		}
	}
	return kept
}

// Attach places every hit on the unit that contains it, per §4.3.6.
//
// Containment is unit.Contains, which is §6.2.1's own predicate, rather than a
// second reading of what a hunk range means. That matters beyond tidiness: a
// hit is attached to the unit a record written from it will be anchored in, and
// §6.2.1 decides with the same function whether a citation lies inside that
// unit. Two predicates would let a hit be attached to one unit and graded
// against another.
//
// Only a RIGHT-side unit can hold a hit. §2.6.1.1 evaluates the added and
// modified RIGHT-side lines and never the removed ones, so every hit is
// numbered on §9.2's RIGHT and belongs in the unit whose hunks those lines came
// from. §6.2.1 compares no side — it works entirely in head coordinates, where
// a deletion is an insertion point — so without this the hit could also land on
// a LEFT unit of the same file, asking a role to judge deleted code against a
// standard about a line the change added.
//
// A hit no unit contains is attached to none, and is not an error. §2.6.1.1
// evaluates over the round's own hunks, so every hit comes from a hunk that
// became a unit and the case does not arise from a diff; and §2.6.1.6 appends
// every hit to `rule-stats.ndjson` whatever becomes of it here, so an
// unattached hit is not a hit that disappeared. What must not happen is the
// other thing: a hit handed to a unit that does not contain it sends the role
// looking at code the match was never in, and §1.6's economy pays for that in
// the author's trust.
func Attach(units []unit.Unit, hits []Hit) []Attachment {
	attached := make([]Attachment, 0, len(units))
	for at := range units {
		formed := &units[at]
		inside := make([]Hit, 0)
		for _, hit := range hits {
			if formed.Side == git.Right && formed.Contains(hit.Path, hit.Line) {
				inside = append(inside, hit)
			}
		}
		attached = append(attached, Attachment{Unit: formed.ID, Hits: inside})
	}
	return attached
}
