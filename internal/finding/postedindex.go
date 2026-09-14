package finding

import (
	"fmt"
	"slices"

	"github.com/deligoez/cr/internal/state"
)

// PostedEntry is one line of `posted-index.ndjson`: a record that reached
// `posted`, held under §7.4.1's key.
//
// §9.3.6 keys the index "exactly as a §7.4.1 waiver is", so the key is
// WaiverKey itself rather than a second spelling of the same four fields —
// which is the reason WaiverKey lives beside the anchor rather than beside the
// waiver store. One construction, WaiverKeyOf, fills the waiver §7.2 writes,
// the entry appended here, and the lookup §6.5.1 drops against, so a fifth
// field added to that key cannot reach one of the three and miss the others.
//
// The record and the round beside it are provenance and never key. What §9.3.6
// asks is whether this defect at this unchanged code has already been sent, and
// a key carrying the id would answer no every time, because §6.1.1 mints a
// fresh id in every round.
//
// It deliberately does not embed state.Stamp, for the reason WaiverProvenance
// gives: that pair is what §9.3.5 has every reader filter on, and §9.3.5 exempts
// this file in as many words. Carrying the round under the name every
// round-scoped reader recognises would invite exactly the read the exemption
// forbids — and ReadStamped refuses `posted-index.ndjson` by name, because
// §2.3.3 does not list it.
type PostedEntry struct {
	// Record is the id of the record that was posted, in the round that
	// posted it. It is what a reader follows back to the comment the
	// author received.
	Record string `json:"record"`
	// WaiverKey is §9.3.6's key.
	WaiverKey
	// Round is the round the record was posted in.
	Round int `json:"round"`
	// Head is the commit the posted record was anchored against.
	Head string `json:"head"`
	// Review is the node id of the review the record reached the author
	// in, which is what keeps `cr post --reconcile` from adopting for a
	// later round a review an earlier round already became. It is empty
	// on an entry written before the field existed.
	Review string `json:"review"`
}

// PostedEntryFor is the index entry one posted record calls for, sent in the
// review whose node id is review.
//
// Both halves come off the one record, as WaiverFor takes both of a waiver's,
// so an entry cannot end up keyed by one record and pointing at another. trees
// are the round's, which WaiverKeyOf reads the anchored lines from.
func PostedEntryFor(trees Trees, record *Finding, review string) (PostedEntry, error) {
	key, err := WaiverKeyOf(trees, record)
	if err != nil {
		return PostedEntry{}, err
	}
	return PostedEntry{
		Record:    record.ID,
		WaiverKey: key,
		Round:     record.Round,
		Head:      record.Head,
		Review:    review,
	}, nil
}

// PostedIndex returns every entry `posted-index.ndjson` holds for one pull
// request. It takes no lock, per §2.3.2.
//
// It is read whole and across rounds, which is §9.3.5's exemption doing the
// only work it exists to do: a comment the author received in round 1 must not
// be raised again in round 2, and an index narrowed to the current round would
// always be empty at the moment §6.5.1 consults it.
func PostedIndex(l state.Layout, owner, repo string, pr int) ([]PostedEntry, error) {
	return state.ReadRecords[PostedEntry](l, owner, repo, pr, state.FilePostedIndex)
}

// AppendPosted adds one entry per record to `posted-index.ndjson` and leaves
// every entry already there.
//
// The caller holds the §2.3.1 lock, and the read below is safe inside it for
// the reason AppendStamped's is: no other writer can be between this read and
// the write that follows it.
//
// An entry already held for the same key is not added a second time. §9.3.6
// holds one entry per record that reached `posted`, and a round that posted a
// record the index already covers is a round in which the drop did not run —
// worth not compounding, and harmless either way, since matching is equality
// over the key.
func AppendPosted(
	k *state.Lock, l state.Layout, owner, repo string, pr int, entries []PostedEntry,
) error {
	held, err := PostedIndex(l, owner, repo, pr)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !slices.ContainsFunc(held, func(e PostedEntry) bool {
			return e.WaiverKey == entry.WaiverKey
		}) {
			held = append(held, entry)
		}
	}
	return state.WriteRecords(k, state.FilePostedIndex, held)
}

// PostedBefore reports the index entry covering a record's key, as WaiverKeyOf
// forms it, if one does.
//
// Matching is equality over the whole of §7.4.1's key, which is WaivedBy's
// comparison and is narrow for the same reason: §7.4.1's hash is over the
// anchored lines and their context, so an entry stops covering a finding the
// moment the code it was posted about, or the code around it, changes. That is
// what lets a real regression in rewritten code be raised again while a comment
// about untouched code is not repeated.
func PostedBefore(index []PostedEntry, key WaiverKey) (PostedEntry, bool) {
	for _, entry := range index {
		if entry.WaiverKey == key {
			return entry, true
		}
	}
	return PostedEntry{}, false
}

// PostedDrops is §9.3.6's report over one round: how many findings the posted
// index silenced, and which already-posted records did the silencing.
//
// It carries no records, for the reason Drops carries none: §9.3.6 drops a
// matching finding "exactly as §6.4.4 drops a waived finding", and §6.4.4 has
// the dropped finding never written to `findings.ndjson` at all. A report
// holding them would put them back within reach of the first caller that
// decided to store them.
//
// The ids it does carry are the earlier records — comments the author already
// has, which they can open on the pull request — and not the findings that were
// dropped.
//
// Both fields are filled rather than nil, per the §12.3 convention.
type PostedDrops struct {
	// Dropped is how many findings §9.3.6 removed. This is the count
	// §6.5.1 writes into the round summary per §10.3.
	Dropped int `json:"dropped"`
	// Posted are the ids of the already-posted records that matched, each
	// named once, in the order they first applied.
	Posted []string `json:"posted"`
}

// Disclosure is §9.3.6's drop count in the shape HonestyDisclosure fixes.
//
// It is printed at zero as well, for the reason Drops.Disclosure is: a count
// that appeared only when it was non-zero would leave a reader unable to tell a
// round in which nothing had been posted before from a round in which the drop
// never ran — and the second is the one worth knowing about, because it sends
// the author a comment they have already read.
func (d PostedDrops) Disclosure() string {
	return fmt.Sprintf("%d finding(s) dropped as already posted, per §9.3.6", d.Dropped)
}

// DropPosted applies §9.3.6 over one round's records: every finding the posted
// index already holds an entry for is taken out, and the removals are counted.
//
// It removes rather than marks, and it is §6.4.4's sentence read again: §9.1's
// table is exhaustive and has no state for a record that was dropped, so a
// record written first and dropped afterwards would sit in `draft` with no
// legal edge out and §10.2.4 would block the round's completeness for as long
// as the pull request lived.
//
// **Run it beside DropWaived and before §6.4.3's marking.** §6.5.1 lists
// §6.4.1, §6.4.2, §6.4.4 and §9.3.6 as one merge's passes, and MarkDuplicates
// writes a representative's id into every other record of its group — so
// dropping a representative afterwards would leave those records naming an id
// nothing in the round holds.
//
// The caller is handed a new slice rather than having its own filtered in
// place, and order is preserved, both for the reasons DropWaived gives.
func DropPosted(trees Trees, records []*Finding, index []PostedEntry) ([]*Finding, PostedDrops, error) {
	kept := make([]*Finding, 0, len(records))
	applied := PostedDrops{Posted: make([]string, 0, len(index))}
	for _, record := range records {
		key, err := WaiverKeyOf(trees, record)
		if err != nil {
			return nil, PostedDrops{}, err
		}
		entry, posted := PostedBefore(index, key)
		if !posted {
			kept = append(kept, record)
			continue
		}
		applied.Dropped++
		if !slices.Contains(applied.Posted, entry.Record) {
			applied.Posted = append(applied.Posted, entry.Record)
		}
	}
	return kept, applied, nil
}
