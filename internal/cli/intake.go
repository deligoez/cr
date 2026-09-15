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

// intakeDrops is one share's two drops as the round summary holds them:
// §6.4.4's and §9.3.6's reports, counts and the waiver and posted ids that
// matched, and never the dropped records themselves.
type intakeDrops struct {
	Waived        finding.Drops       `json:"waived"`
	AlreadyPosted finding.PostedDrops `json:"already_posted"`
}

// intakeKeys is one share's two drops as intake.json holds them: the intake key
// of every record each drop removed.
//
// The keys are what the counts are made of. One finding can reach both
// commands — a role file merged and also recorded, in either order — and each
// drops it again; a count kept per share would add the second drop to the
// first. A set of keys joins them, so one record is one drop however many
// shares took it out. They live in state.FileIntake rather than in the share,
// because §6.4.4 lets only the count of a dropped finding reach the summary.
type intakeKeys struct {
	WaivedKeys []intakeKey `json:"waived_keys"`
	PostedKeys []intakeKey `json:"posted_keys"`
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

// The two kinds of intake key.
const (
	// intakeKindID is a record's id, which §6.1 keeps stable for the life
	// of the pull request.
	intakeKindID = "record_id"
	// intakeKindIdentity is §6.4.1's identity, for a record that carries
	// no id.
	intakeKindIdentity = "identity"
)

// intakeKey is the key a record is counted under in the intake, stating which
// of the two kinds it is.
type intakeKey struct {
	Kind string `json:"kind"`
	// ID is the record id of an intakeKindID key.
	ID string `json:"id,omitempty"`
	// Identity is the §6.4.1 identity of an intakeKindIdentity key.
	Identity *finding.DedupKey `json:"identity,omitempty"`
}

// intakeKeyOf is the key record is counted under.
func intakeKeyOf(record *finding.Finding) intakeKey {
	if record.ID != "" {
		return intakeKey{Kind: intakeKindID, ID: record.ID}
	}
	identity := finding.DedupKeyOf(record)
	return intakeKey{Kind: intakeKindIdentity, Identity: &identity}
}

// member is the key as one member of a set. An id is itself, so it meets the
// ids a merge share and the stored records give; the identity is spelled with
// spaces, which no id holds, so the two never name one another.
func (k intakeKey) member() string {
	if k.Kind == intakeKindIdentity && k.Identity != nil {
		return fmt.Sprintf("%q %s %d %q", k.Identity.Path, k.Identity.Side, k.Identity.Line, k.Identity.Class)
	}
	return k.ID
}

// droppedKeys is the intake key of every record in before that a drop left out
// of kept, in before's order.
func droppedKeys(before, kept []*finding.Finding) []intakeKey {
	left := make(map[*finding.Finding]bool, len(kept))
	for _, record := range kept {
		left[record] = true
	}
	keys := make([]intakeKey, 0)
	for _, record := range before {
		if !left[record] {
			keys = append(keys, intakeKeyOf(record))
		}
	}
	return keys
}

// newMergeIntake is a merge's share of the intake and the keys beside it.
func newMergeIntake(merged *mergeOutcome) (*mergeIntake, intakeKeys) {
	ids := make([]string, 0, len(merged.records))
	for _, record := range merged.records {
		ids = append(ids, record.ID)
	}
	return &mergeIntake{Records: ids, intakeDrops: intakeDrops{Waived: merged.waived, AlreadyPosted: merged.posted}},
		intakeKeys{WaivedKeys: merged.waivedKeys, PostedKeys: merged.postedKeys}
}

// roundIntake is both shares of the round's intake, the keys state.FileIntake
// holds for them, and the digest of the merge output the round summary names.
type roundIntake struct {
	merge mergeIntake
	// mergeKeys are the merge share's keys, and nil for a share with none:
	// one a merge before intake.json wrote.
	mergeKeys  *intakeKeys
	recorded   map[string]recordIntake
	recordKeys map[string]intakeKeys
	mergedHash string
}

// readIntake reads the round's intake. A share no command has written reads as
// empty, and an intake.json or summary.json that is not there as one holding
// nothing. The reads take no lock, per §2.3.2.
func readIntake(l state.Layout, owner, repo string, pr, round int) (*roundIntake, error) {
	in := &roundIntake{}
	var err error
	in.merge, _, err = state.ReadRoundSection[mergeIntake](l, owner, repo, pr, round, state.FileSummary, summaryMergeIntake)
	if err != nil {
		return nil, err
	}
	in.recorded, _, err = state.ReadRoundSection[map[string]recordIntake](
		l, owner, repo, pr, round, state.FileSummary, summaryRecordIntake)
	if err != nil {
		return nil, err
	}
	if in.recorded == nil {
		in.recorded = make(map[string]recordIntake)
	}
	in.mergedHash, _, err = state.ReadRoundSection[string](l, owner, repo, pr, round, state.FileSummary, summaryMergedHash)
	if err != nil {
		return nil, err
	}
	mergeKeys, kept, err := state.ReadRoundSection[intakeKeys](
		l, owner, repo, pr, round, state.FileIntake, summaryMergeIntake)
	if err != nil {
		return nil, err
	}
	if kept {
		in.mergeKeys = &mergeKeys
	}
	in.recordKeys, _, err = state.ReadRoundSection[map[string]intakeKeys](
		l, owner, repo, pr, round, state.FileIntake, summaryRecordIntake)
	if err != nil {
		return nil, err
	}
	if in.recordKeys == nil {
		in.recordKeys = make(map[string]intakeKeys)
	}
	return in, nil
}

// writeIntakeCounts is ownerIntake's section: raised, waived and already posted,
// over both shares of the intake and the round's stored records.
func writeIntakeCounts(held *state.Lock, round int, in *roundIntake, stored []*finding.Finding) error {
	raised, waived, posted := intakeTotals(in, stored)
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
// A share intake.json holds no keys for was written before the keys were, and
// is added by count, as it was then: its drops add to waived and already posted,
// and to raised unless it is the last merge's output recorded. Such a round
// keeps the counts it had rather than reading 0 for drops it cannot name.
//
// The recorded entries are added in digest order and the waiver and posted ids
// joined without repeats, so the same state gives the same document.
func intakeTotals(in *roundIntake, stored []*finding.Finding) (raised int, waived finding.Drops, posted finding.PostedDrops) {
	keys := make(map[string]bool, len(stored)+len(in.merge.Records))
	for _, record := range stored {
		keys[intakeKeyOf(record).member()] = true
	}
	for _, id := range in.merge.Records {
		keys[id] = true
	}
	waived = finding.Drops{Waivers: make([]string, 0)}
	posted = finding.PostedDrops{Posted: make([]string, 0)}
	totals := intakeSets{waived: make(map[string]bool), posted: make(map[string]bool)}
	totals.add(&waived, &posted, &in.merge.intakeDrops, in.mergeKeys, true)
	inputs := slices.Collect(maps.Keys(in.recorded))
	for input := range in.recordKeys {
		if _, held := in.recorded[input]; !held {
			inputs = append(inputs, input)
		}
	}
	slices.Sort(inputs)
	for _, input := range inputs {
		entry := in.recorded[input]
		if entry.Merged && input != in.mergedHash {
			continue
		}
		var shareKeys *intakeKeys
		if held, kept := in.recordKeys[input]; kept {
			shareKeys = &held
		}
		totals.add(&waived, &posted, &entry.intakeDrops, shareKeys, !entry.Merged)
	}
	waived.Dropped = len(totals.waived) + totals.waivedCount
	posted.Dropped = len(totals.posted) + totals.postedCount
	maps.Copy(keys, totals.waived)
	maps.Copy(keys, totals.posted)
	return len(keys) + totals.raisedCount, waived, posted
}

// intakeSets is the running union intakeTotals builds: the keys of every drop,
// and the counts of the shares that carry no keys.
type intakeSets struct {
	waived, posted                        map[string]bool
	waivedCount, postedCount, raisedCount int
}

// add adds one share into the totals: its waiver and posted ids into the two
// reports, and its keys into the two sets, or its counts when keys is nil. A
// share's counts reach raised only when raises says its drops are not already
// among another share's.
func (s *intakeSets) add(
	waived *finding.Drops, posted *finding.PostedDrops, share *intakeDrops, keys *intakeKeys, raises bool,
) {
	for _, id := range share.Waived.Waivers {
		if !slices.Contains(waived.Waivers, id) {
			waived.Waivers = append(waived.Waivers, id)
		}
	}
	for _, id := range share.AlreadyPosted.Posted {
		if !slices.Contains(posted.Posted, id) {
			posted.Posted = append(posted.Posted, id)
		}
	}
	if keys == nil {
		s.waivedCount += share.Waived.Dropped
		s.postedCount += share.AlreadyPosted.Dropped
		if raises {
			s.raisedCount += share.Waived.Dropped + share.AlreadyPosted.Dropped
		}
		return
	}
	for _, key := range keys.WaivedKeys {
		s.waived[key.member()] = true
	}
	for _, key := range keys.PostedKeys {
		s.posted[key.member()] = true
	}
}
