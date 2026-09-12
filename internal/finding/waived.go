package finding

import (
	"fmt"
	"slices"
)

// Drops is §6.4.4's report over one round: how many findings an active waiver
// silenced, and which waivers did the silencing.
//
// It is a count and a list of waiver ids, and deliberately not a list of the
// records. §6.4.4 has a dropped finding never written to `findings.ndjson` at
// all, so that no record ever exists in a state §9.1 does not define, and only
// the count reaches the round summary — a report carrying the records would put
// them back within reach of the first caller that decided to store them, which
// is the whole of what that sentence forbids.
//
// The waiver ids are not the records. They name silences the reviewer already
// wrote and `cr waivers list` already prints, so a reader asking which of their
// decisions is still in force has a way through §7.4.7 rather than through a
// finding cr kept a copy of.
//
// Both fields are filled rather than nil, per the §12.3 convention.
type Drops struct {
	// Dropped is how many findings §6.4.4 removed. This is the count
	// §10.3's round summary takes and the one §10.1.6 reports.
	Dropped int `json:"dropped"`
	// Waivers are the ids of the waivers that matched, each named once, in
	// the order they first applied.
	Waivers []string `json:"waivers"`
}

// Disclosure is §10.1.6's waiver count, one of the seven reports §11.1 exempts
// from `--quiet`, in the shape HonestyDisclosure fixes.
//
// It is printed at zero as well, for the reason Overlaps.Disclosure is: a count
// that appeared only when it was non-zero would leave a reader unable to tell a
// round in which no waiver matched from a round in which the drop never ran —
// and the second is the one worth knowing about, because a merge that silently
// applied no waiver still produces a draft that looks complete while re-raising
// what the reviewer set aside.
func (d Drops) Disclosure() string {
	return fmt.Sprintf("%d finding(s) dropped by %d active waiver(s), per §6.4.4",
		d.Dropped, len(d.Waivers))
}

// DropWaived applies §6.4.4 over one round's records: every finding an active
// waiver covers is taken out, and the removals are counted.
//
// It removes rather than marks, and that is the whole of the rule. §6.4.3 does
// the opposite for a duplicate — the record is retained in state `duplicate`
// with `duplicate_of` naming the representative — and §6.4.4 says the opposite
// in as many words, because §9.1's transition table is exhaustive and has no
// `waived` state to move a record into. Round 3's `waived-record-no-state`
// found the gap: a record dropped after it was written would sit in `draft`
// with no legal edge out, and §10.2.4 would then block the round's completeness
// for as long as the pull request lived.
//
// So the records that leave here are the records that get written, and a waived
// one is not among them. The caller is handed a new slice rather than having
// its own filtered in place: the pointers it still holds are the only remaining
// reference to a dropped record, and a function that had shortened the caller's
// slice would leave it believing the drop had reached further than it had.
//
// **Run it before §6.4.3's marking, not after.** MarkDuplicates writes a
// representative's id into every other record of its group, so dropping a
// representative afterwards would leave those records naming an id nothing in
// the round holds. §6.5.1 lists the passes in that order for this reason.
//
// waivers are §7.4.4's two files together, as ActiveWaivers reads them: the
// repository-wide file and the pull request's own. Matching is WaivedBy's,
// which is equality over the whole of §7.4.1's key — so §7.4.2's narrowness
// holds here, and a waiver stops silencing the moment the anchored code
// changes.
//
// Order is preserved, so the merged file still reads in the order the role
// files were read.
func DropWaived(records []*Finding, waivers []WaiverRecord) ([]*Finding, Drops) {
	kept := make([]*Finding, 0, len(records))
	applied := Drops{Waivers: make([]string, 0, len(waivers))}
	for _, record := range records {
		waiver, waived := WaivedBy(waivers, record)
		if !waived {
			kept = append(kept, record)
			continue
		}
		applied.Dropped++
		if !slices.Contains(applied.Waivers, waiver.ID) {
			applied.Waivers = append(applied.Waivers, waiver.ID)
		}
	}
	return kept, applied
}
