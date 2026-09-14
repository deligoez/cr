package cli

import (
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

// mergeIntake is summaryMergeIntake's shape.
//
// It carries record ids only for the records the output file holds, never for
// the ones a drop took out: §6.4.4 lets only the count of those reach the round
// summary.
type mergeIntake struct {
	// Records are the ids of the records the merge's output file holds.
	Records []string `json:"records"`
	// Waived and AlreadyPosted are the merge's §6.4.4 and §9.3.6 drops.
	Waived        finding.Drops       `json:"waived"`
	AlreadyPosted finding.PostedDrops `json:"already_posted"`
}

// recordIntake is one entry of summaryRecordIntake: one file `cr record` read.
type recordIntake struct {
	// Merged is whether the file was the output `cr merge` last wrote for
	// the round when it was recorded. Such a file's records are already
	// counted in that merge's share, so only its drops are added, and only
	// while that merge is still the round's last: a later merge applies the
	// same drops to the same role files itself.
	Merged bool `json:"merged"`
	// Waived and AlreadyPosted are the drops `cr record` applied to the file.
	Waived        finding.Drops       `json:"waived"`
	AlreadyPosted finding.PostedDrops `json:"already_posted"`
}

// newMergeIntake is a merge's share of the intake.
func newMergeIntake(merged *mergeOutcome) *mergeIntake {
	ids := make([]string, 0, len(merged.records))
	for _, record := range merged.records {
		ids = append(ids, record.ID)
	}
	return &mergeIntake{Records: ids, Waived: merged.waived, AlreadyPosted: merged.posted}
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

// intakeTotals adds the two shares up.
//
// Raised is every record the round holds or the last merge's output holds, each
// id once, and every finding a drop took out before it could be stored. A drop
// `cr record` applied to the last merge's own output is not added to raised a
// second time, because that output's records are already among the ids; and a
// drop it applied to an earlier merge's output is left out altogether, because
// the last merge applied its drops to the same role files again.
//
// The recorded entries are added in digest order and the waiver and posted ids
// joined without repeats, so the same state gives the same document.
func intakeTotals(
	merge *mergeIntake, recorded map[string]recordIntake, mergedHash string, stored []*finding.Finding,
) (raised int, waived finding.Drops, posted finding.PostedDrops) {
	ids := make(map[string]bool, len(stored)+len(merge.Records))
	for _, record := range stored {
		ids[record.ID] = true
	}
	for _, id := range merge.Records {
		ids[id] = true
	}
	raised = len(ids) + merge.Waived.Dropped + merge.AlreadyPosted.Dropped
	waived = finding.Drops{Waivers: make([]string, 0)}
	posted = finding.PostedDrops{Posted: make([]string, 0)}
	addDrops(&waived, &posted, merge.Waived, merge.AlreadyPosted)
	for _, input := range slices.Sorted(maps.Keys(recorded)) {
		entry := recorded[input]
		if entry.Merged && input != mergedHash {
			continue
		}
		if !entry.Merged {
			raised += entry.Waived.Dropped + entry.AlreadyPosted.Dropped
		}
		addDrops(&waived, &posted, entry.Waived, entry.AlreadyPosted)
	}
	return raised, waived, posted
}

// addDrops adds one share's two drops into the totals.
func addDrops(waived *finding.Drops, posted *finding.PostedDrops, w finding.Drops, p finding.PostedDrops) {
	waived.Dropped += w.Dropped
	for _, id := range w.Waivers {
		if !slices.Contains(waived.Waivers, id) {
			waived.Waivers = append(waived.Waivers, id)
		}
	}
	posted.Dropped += p.Dropped
	for _, id := range p.Posted {
		if !slices.Contains(posted.Posted, id) {
			posted.Posted = append(posted.Posted, id)
		}
	}
}
