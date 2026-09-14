package cli

import (
	"fmt"
	"maps"
	"slices"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// ownerIntake is the head of §10.3's list — raised, waived and already posted —
// which has two writers of one computation, for the reason ownerDiscards does.
//
// `cr merge` is not the only door a record comes in by: §11 lists
// `cr record <pr> <file>` with no precondition, and a single role's file handed
// straight to it is recorded, and its waived findings dropped, with no merge
// run at all. A count only `cr merge` wrote would read nothing for such a round,
// and a merge run after it would replace the count with its own. So both
// commands keep their own share of the intake — summaryMergeIntake and
// summaryRecordIntake — and both write these three through writeIntakeCounts,
// over both shares and the round's stored records, under the same lock.
const ownerIntake summaryOwner = "cr merge and cr record"

// The two shares ownerIntake's counts are computed from.
const (
	// summaryMergeIntake is `cr merge`'s share: the ids of the records its
	// output holds, and what its two drops took out of the per-role files.
	// It is replaced whole by every merge, as the output file is.
	summaryMergeIntake = "merge_intake"
	// summaryRecordIntake is `cr record`'s share: what each run's two drops
	// took out of the file it read, keyed by §1.4's normalised hash of that
	// file, so recording one file again replaces its entry rather than
	// adding to it.
	summaryRecordIntake = "record_intake"
)

// intakeDrops is what one share's two drops took out: §6.4.4's and §9.3.6's
// reports, and the intake key of every record each drop removed.
//
// The keys are what the counts are made of. One finding can reach both
// commands — a role file merged and also recorded, in either order — and each
// drops it again; a count kept per share would add the second drop to the
// first. A set of keys joins them, so one record is one drop however many
// shares took it out.
type intakeDrops struct {
	Waived        finding.Drops       `json:"waived"`
	WaivedKeys    []string            `json:"waived_keys"`
	AlreadyPosted finding.PostedDrops `json:"already_posted"`
	PostedKeys    []string            `json:"posted_keys"`
}

// mergeIntake is summaryMergeIntake's shape.
type mergeIntake struct {
	// Records are the ids of the records the merge's output file holds.
	Records []string `json:"records"`
	// intakeDrops are the merge's §6.4.4 and §9.3.6 drops.
	intakeDrops
}

// recordIntake is one entry of summaryRecordIntake: one file `cr record` read.
type recordIntake struct {
	// Merged is whether the file was the output `cr merge` last wrote for
	// the round when it was recorded. Such a file's drops are counted only
	// while that merge is still the round's last: a later merge applies the
	// same drops to the same role files itself.
	Merged bool `json:"merged"`
	// intakeDrops are the drops `cr record` applied to the file.
	intakeDrops
}

// intakeKey is the key a record is counted under in the intake: its id, which
// §6.1 keeps stable for the life of the pull request, or §6.4.1's identity for
// a record that carries none. The identity is spelled with spaces, which no
// id holds, so the two never name one another.
func intakeKey(record *finding.Finding) string {
	if record.ID != "" {
		return record.ID
	}
	key := finding.DedupKeyOf(record)
	return fmt.Sprintf("%q %s %d %q", key.Path, key.Side, key.Line, key.Class)
}

// droppedKeys is the intake key of every record in before that a drop left out
// of kept, in before's order.
func droppedKeys(before, kept []*finding.Finding) []string {
	left := make(map[*finding.Finding]bool, len(kept))
	for _, record := range kept {
		left[record] = true
	}
	keys := make([]string, 0)
	for _, record := range before {
		if !left[record] {
			keys = append(keys, intakeKey(record))
		}
	}
	return keys
}

// newMergeIntake is a merge's share of the intake.
func newMergeIntake(merged *mergeOutcome) *mergeIntake {
	ids := make([]string, 0, len(merged.records))
	for _, record := range merged.records {
		ids = append(ids, record.ID)
	}
	return &mergeIntake{Records: ids, intakeDrops: intakeDrops{
		Waived: merged.waived, WaivedKeys: merged.waivedKeys,
		AlreadyPosted: merged.posted, PostedKeys: merged.postedKeys,
	}}
}

// readIntake reads both shares of the round's intake and the digest of the
// merge output the round summary names. A share no command has written reads
// as empty. The reads take no lock, per §2.3.2.
func readIntake(
	l state.Layout, owner, repo string, pr, round int,
) (merge mergeIntake, recorded map[string]recordIntake, mergedHash string, err error) {
	merge, _, err = state.ReadRoundSection[mergeIntake](l, owner, repo, pr, round, state.FileSummary, summaryMergeIntake)
	if err != nil {
		return merge, nil, "", err
	}
	recorded, _, err = state.ReadRoundSection[map[string]recordIntake](
		l, owner, repo, pr, round, state.FileSummary, summaryRecordIntake)
	if err != nil {
		return merge, nil, "", err
	}
	if recorded == nil {
		recorded = make(map[string]recordIntake)
	}
	// An entry a summary kept before the keys were has none, and `cr record`
	// writes every entry back: an empty list, never null.
	for input, entry := range recorded {
		if entry.WaivedKeys == nil {
			entry.WaivedKeys = make([]string, 0)
		}
		if entry.PostedKeys == nil {
			entry.PostedKeys = make([]string, 0)
		}
		recorded[input] = entry
	}
	mergedHash, _, err = state.ReadRoundSection[string](l, owner, repo, pr, round, state.FileSummary, summaryMergedHash)
	return merge, recorded, mergedHash, err
}

// writeIntakeCounts is ownerIntake's section: raised, waived and already posted,
// over both shares of the intake and the round's stored records.
func writeIntakeCounts(
	held *state.Lock, round int, merge *mergeIntake, recorded map[string]recordIntake, mergedHash string,
	stored []*finding.Finding,
) error {
	raised, waived, posted := intakeTotals(merge, recorded, mergedHash, stored)
	return writeSummary(held, round, ownerIntake, []summaryCount{
		{key: summaryRaised, value: raised},
		{key: summaryWaived, value: waived},
		{key: summaryAlreadyPosted, value: posted},
	})
}

// intakeTotals adds the two shares up, by key.
//
// Raised is every record the round holds, the last merge's output holds, or a
// drop took out before it could be stored, each key once. Waived and already
// posted are the keys each drop took out, each once across every share, so a
// finding dropped by `cr merge` and by `cr record` alike is one drop. A drop
// `cr record` applied to an earlier merge's output is left out altogether,
// because the last merge applied its drops to the same role files again.
//
// The recorded entries are added in digest order and the waiver and posted ids
// joined without repeats, so the same state gives the same document.
func intakeTotals(
	merge *mergeIntake, recorded map[string]recordIntake, mergedHash string, stored []*finding.Finding,
) (raised int, waived finding.Drops, posted finding.PostedDrops) {
	keys := make(map[string]bool, len(stored)+len(merge.Records))
	for _, record := range stored {
		keys[intakeKey(record)] = true
	}
	for _, id := range merge.Records {
		keys[id] = true
	}
	waived = finding.Drops{Waivers: make([]string, 0)}
	posted = finding.PostedDrops{Posted: make([]string, 0)}
	waivedKeys, postedKeys := make(map[string]bool), make(map[string]bool)
	addDrops(&waived, &posted, waivedKeys, postedKeys, &merge.intakeDrops)
	for _, input := range slices.Sorted(maps.Keys(recorded)) {
		entry := recorded[input]
		if entry.Merged && input != mergedHash {
			continue
		}
		addDrops(&waived, &posted, waivedKeys, postedKeys, &entry.intakeDrops)
	}
	waived.Dropped, posted.Dropped = len(waivedKeys), len(postedKeys)
	maps.Copy(keys, waivedKeys)
	maps.Copy(keys, postedKeys)
	return len(keys), waived, posted
}

// addDrops adds one share's two drops into the totals: its keys into the two
// sets and its waiver and posted ids into the two reports.
func addDrops(
	waived *finding.Drops, posted *finding.PostedDrops, waivedKeys, postedKeys map[string]bool, share *intakeDrops,
) {
	for _, key := range share.WaivedKeys {
		waivedKeys[key] = true
	}
	for _, id := range share.Waived.Waivers {
		if !slices.Contains(waived.Waivers, id) {
			waived.Waivers = append(waived.Waivers, id)
		}
	}
	for _, key := range share.PostedKeys {
		postedKeys[key] = true
	}
	for _, id := range share.AlreadyPosted.Posted {
		if !slices.Contains(posted.Posted, id) {
			posted.Posted = append(posted.Posted, id)
		}
	}
}
