package cli

import (
	"fmt"
	"maps"
	"slices"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// summaryOwner is the command §10.3 makes responsible for one of the round
// summary's counts.
//
// §10.3 names three writers and round 8's unassigned-writer adds the fourth.
// `cr record` is on the list because it is what produces the two counts nothing
// else can: §6.4.3 has it stamp `duplicate` from the `duplicate_of` `cr merge`
// emitted, and §3.5.4's `suppressed` from the thread the agent named. §6.5.1
// keeps `state` out of `cr merge`'s output entirely, so the merge never writes
// either one; and by the time `cr draft` and `cr post` read the round, both
// retirements have already happened and neither command can tell a record it
// never saw from a record that was never raised.
type summaryOwner string

// The four writers of `rounds/<n>/summary.json`, named as the command a reader
// would run.
const (
	ownerMerge  summaryOwner = "cr merge"
	ownerRecord summaryOwner = "cr record"
	ownerDraft  summaryOwner = "cr draft"
	ownerPost   summaryOwner = "cr post"
	// ownerDiscards is §7.2's two discard counts, which have two writers
	// of one computation rather than one writer. §7.2 has `cr draft` and
	// `cr post` both read the discard verbs out of draft.md and both
	// store what they read, so a block deleted and then posted with no
	// redraft is stored as discarded by `cr post --confirm` alone — and a
	// count only `cr draft` wrote would go on reading the last rendering's
	// number beside a findings.ndjson that says otherwise. Both commands
	// write this section through discardCounts, over the records they
	// store under the same lock, so the count is the stored records'
	// whichever of the two ran last.
	ownerDiscards summaryOwner = "cr draft and cr post"
	// ownerForcing is summaryForcedRecords, which has two writers for the
	// reason ownerDiscards does: §6.3.1 forces at record time and again at
	// draft time, and each moment stores the records it moved, so each is
	// the only one that knows which it moved. Both write the section
	// through keepForced.
	ownerForcing summaryOwner = "cr record and cr draft"
)

// The keys `rounds/<n>/summary.json` holds §10.3's counts under. Three more are
// declared beside the code that writes them, where they were needed first:
// summaryWaived, summaryAlreadyPosted, summaryForcedToQuestion and
// summaryNewClasses. summaryOwners below is where all of them are gathered.
const (
	// summaryRaised is how many findings the per-role files handed
	// `cr merge`, before any of §6.5.1's passes removed one. It is the
	// denominator every later count is read against.
	summaryRaised = "raised"
	// summaryDeduplicated is how many records §6.4.3 retired as
	// duplicates, and summarySuppressedByThread how many §3.5.4's
	// ingested human threads already covered.
	summaryDeduplicated       = "deduplicated"
	summarySuppressedByThread = "suppressed_by_thread"
	// summaryDrafted is how many records §7.1 rendered into the round's
	// draft, which is also how many §9.1 holds in `queued`.
	summaryDrafted = "drafted"
	// summaryDiscardedNotHere and summaryDiscardedWrong are §7.2's two
	// discard verbs as §7.4.2 dispositions them, counted apart because
	// §7.3.4 counts only one of them against a class.
	summaryDiscardedNotHere = "discarded_not_here"
	summaryDiscardedWrong   = "discarded_wrong"
	// summaryComments is §1.6.2's comment count against post.max_comments,
	// and summaryProbeCap §5.6.4's probe cap over the round.
	summaryComments = "comments"
	summaryProbeCap = "probe_cap"
	// summaryPosted is how many of the round's records reached the author,
	// and summaryPayloadHash §8.3.3's hash of the payload they reached
	// them in.
	//
	// They are §10.3's finalisation, written only once `cr post --confirm`
	// has built a payload and sent it, or `--reconcile` has adopted the
	// review that carried it — and absent otherwise, never null and never
	// the dry run's hash. A round drafted and never posted, or run only
	// through §8.5.1's dry run, sent no payload, so there is no hash that
	// could honestly stand there: a dry-run hash would name a review nobody
	// received, and a null would read as a count cr failed to take. Such a
	// round keeps a summary without either key, and that unfinalised
	// summary is a valid terminal state for it rather than a gap.
	summaryPosted      = "posted"
	summaryPayloadHash = "payload_hash"
	// summaryConfirmGiven is §8.5.4's one fact about the gate: that
	// `--confirm` was given. It is written with the two above and absent
	// on the same rounds, and it is the whole of what the summary says
	// about the gate — §8.5.4 records that and the payload hash and nothing
	// more, so no key here says a human read the draft or that draft.md
	// changed.
	summaryConfirmGiven = "confirm_given"
	// summaryRecordedAt is when `cr record` last recorded the round, a
	// moment it leaves in every round it records. §2.6.3.4's window orders
	// rounds by moment, and a round recorded with no record naming a rule
	// and no triage event leaves no other: without it such a round could
	// only be placed at a bound, older than it is, and the newest clean
	// round would fall out of the window it closes. It is written on every
	// run with the rest of the section, so it is the latest recording.
	summaryRecordedAt = "recorded_at"
	// summaryForcedRecords is the ids of the round's records §6.3.1 has
	// moved from finding to question, which §6.3.2's count per class is
	// taken over. It is kept because the move is stored: once a record
	// the forcing moved is in findings.ndjson it reads as a question
	// exactly as one its role wrote as a question does, and §6.3.2 counts
	// only the first.
	summaryForcedRecords = "forced_records"
)

// summaryOwners is §10.3's writer list: every count the round summary holds,
// and the command that writes it.
//
// It is a table rather than four scattered call sites because §10.3 gives one
// document four writers, and the failure that invites is a count nobody owns —
// which is exactly round 8's unassigned-writer. A key here has a command beside
// it, and writeSummary refuses a command that reaches for a key it was not
// given, so the assignment is enforced where it is stated instead of being a
// comment two files away from the write.
var summaryOwners = map[string]summaryOwner{
	summaryRaised:             ownerMerge,
	summaryWaived:             ownerMerge,
	summaryAlreadyPosted:      ownerMerge,
	summaryMergedHash:         ownerMerge,
	summaryDeduplicated:       ownerRecord,
	summarySuppressedByThread: ownerRecord,
	summaryRecordedAt:         ownerRecord,
	summaryForcedRecords:      ownerForcing,
	summaryForcedToQuestion:   ownerDraft,
	summaryForcedByRetraction: ownerDraft,
	summaryNewClasses:         ownerDraft,
	summaryDrafted:            ownerDraft,
	summaryDiscardedNotHere:   ownerDiscards,
	summaryDiscardedWrong:     ownerDiscards,
	summaryComments:           ownerDraft,
	summaryProbeCap:           ownerDraft,
	summaryPosted:             ownerPost,
	summaryPayloadHash:        ownerPost,
	summaryConfirmGiven:       ownerPost,
}

// summaryCap is a cap's state as the round summary records it: what the round
// has spent, and what it was measured against.
//
// §1.6.2's comment cap and §5.6.4's probe cap are the same shape and are
// recorded in the same one, so a reader of the document does not have to learn
// two spellings of one idea. It is a type of its own rather than either cap's
// own struct because those are decisions — finding.CommentCap refuses, and
// probe.RoundCap refuses — and what reaches the document is the pair of numbers
// the decision was made from.
type summaryCap struct {
	// Count is what the round has spent: comments queued, or probes run.
	Count int `json:"count"`
	// Max is the §2.7 setting it was measured against.
	Max int `json:"max"`
}

// summaryCount is one of §10.3's counts as a run computed it: the key
// summary.json holds it under, and the value.
type summaryCount struct {
	key   string
	value any
}

// discardCounts is ownerDiscards' section: the round's records §7.2's discard
// verbs have moved to `discarded`, by §7.4.2's disposition.
//
// It is taken from the records about to be stored rather than from what this
// run triaged. A regeneration reads the draft it replaces, so a block deleted
// in the first run is already `discarded` by the second and appears in no
// triage the second run makes — a count of this run's discards would read one,
// then zero, for one reviewer decision.
func discardCounts(records []*finding.Finding) []summaryCount {
	notHere, wrong := 0, 0
	for _, record := range records {
		if record.State != finding.StateDiscarded {
			continue
		}
		switch record.Disposition {
		case finding.DispositionNotHere:
			notHere++
		case finding.DispositionWrong:
			wrong++
		}
	}
	return []summaryCount{
		{key: summaryDiscardedNotHere, value: notHere},
		{key: summaryDiscardedWrong, value: wrong},
	}
}

// forcedRecords is the round's summaryForcedRecords: the ids an earlier moment
// of §6.3.1 moved, and none for a round no command has kept any for. The read
// takes no lock, per §2.3.2.
func forcedRecords(l state.Layout, round *state.Meta) ([]string, error) {
	kept, _, err := state.ReadRoundSection[[]string](
		l, round.Owner, round.Repo, round.PR, round.Round, state.FileSummary, summaryForcedRecords)
	return kept, err
}

// keepForced is ownerForcing's section: the ids the round's forcing has moved,
// the ones moved this run joined to the ones already kept, sorted and without
// repeats.
//
// The kept ids are read again here, under the caller's lock, rather than taken
// from the read the run counted with. A section is replaced whole, so an id
// another run kept between that read and this write would otherwise be dropped,
// and the record it names would stop being counted as forced.
func keepForced(held *state.Lock, l state.Layout, round *state.Meta, moved []string) error {
	kept, err := forcedRecords(l, round)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(kept)+len(moved))
	ids = append(append(ids, kept...), moved...)
	slices.Sort(ids)
	return writeSummary(held, round.Round, ownerForcing, []summaryCount{
		{key: summaryForcedRecords, value: slices.Compact(ids)},
	})
}

// writeSummary replaces owner's whole share of the round's summary.json and
// touches nothing else.
//
// Each count goes through state.UpdateRoundSection, which leaves every field
// this writer does not own byte for byte. That is the whole of what makes one
// document with four writers work: a command that decoded summary.json into a
// struct of its own fields would re-encode it without every field that struct
// does not name, and the history §10.3 exists to make reconstructable from
// state alone would lose whatever the writer before it put there.
//
// A section is replaced, never added to, and that is round 8's
// non-idempotent-accumulation. None of the writers runs once per round: §7.1.6
// regenerates the draft as often as the reviewer likes, §8.5.1 makes a dry run
// the ordinary precursor to `cr post --confirm`, and re-running `cr merge`
// after fixing one role's output is the obvious recovery. A count added to on
// each run would inflate the waived, deduplicated and drafted counts, and the
// round summary would be the one place a re-run silently changed the record.
// So every writer computes its counts from the round's state and writes all of
// them on every run, and completeOwnership refuses a run that leaves one out —
// a count kept from an earlier run would be a count this run did not measure.
//
// The lock is the caller's, because every one of the four writers is already
// holding §2.3.1's lock for its own artefacts when it gets here, and the round
// summary has to land beside them rather than in a second window.
func writeSummary(held *state.Lock, round int, owner summaryOwner, counts []summaryCount) error {
	if err := completeOwnership(owner, counts); err != nil {
		return err
	}
	for _, count := range counts {
		if err := state.UpdateRoundSection(
			held, round, state.FileSummary, count.key, count.value,
		); err != nil {
			return err
		}
	}
	return nil
}

// completeOwnership holds a writer to exactly its own share of §10.3's list:
// every count it passes is one summaryOwners gives it, no count is passed
// twice, and every count summaryOwners gives it is passed.
//
// The refusal is not defensive noise. §10.3's document is the one place two
// commands could write the same key with different meanings, and the result
// would be a history that reads as complete and is not — a count silently
// overwritten by a command that had no business computing it, or a count left
// standing from a run whose inputs no longer hold. There is no wording in a
// call site that catches either; a table read at the write is.
//
// The missing count named is the first by key, so the same fault is named the
// same way on every run, as §2.1.1 asks.
func completeOwnership(owner summaryOwner, counts []summaryCount) error {
	seen := make(map[string]bool, len(counts))
	for _, count := range counts {
		at, listed := summaryOwners[count.key]
		if !listed {
			return fmt.Errorf(
				"%q is not one of §10.3's round summary counts, so %s may not write it",
				count.key, owner)
		}
		if at != owner {
			return fmt.Errorf("§10.3 gives %q to %s, so %s may not write it",
				count.key, at, owner)
		}
		if seen[count.key] {
			return fmt.Errorf("%s writes %q twice in one run", owner, count.key)
		}
		seen[count.key] = true
	}
	for _, key := range slices.Sorted(maps.Keys(summaryOwners)) {
		if summaryOwners[key] == owner && !seen[key] {
			return fmt.Errorf(
				"§10.3 gives %q to %s, and this run did not write it: a section is replaced "+
					"whole on every run, so a count left out would keep an earlier run's value",
				key, owner)
		}
	}
	return nil
}
