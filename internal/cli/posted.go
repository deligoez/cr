package cli

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/state"
)

// postedThreads is the field §8.3.3's returned thread ids are added to
// posted.json under, once the network call has returned them.
//
// It is a field of the same document rather than a file of its own because
// §2.3's table gives a round one posted.json and calls it "the exact payload
// posted for round n": the ids GitHub assigned are what that payload became,
// and a second file would let the two disagree about which round posted what.
const postedThreads = "threads"

// postedRecords is the field the ids of the records the payload was drawn from
// are written to posted.json under.
//
// It is in the same document and for the same reason postedThreads is: §2.3's
// table gives a round one posted.json. What it buys is §8.4.4's adoption —
// Comment.Record is `json:"-"` because GitHub has no field for it, so a payload
// read back off disk names no record, and a reconciliation without this would
// have to guess which records reached the author from the comments' anchors.
const postedRecordsSection = "records"

// recordSentRecords writes the ids the payload's comments were drawn from into
// posted.json, beside the payload.
//
// It runs before every send, beside `post_unresolved`, because any call whose
// records are not marked posted afterwards — an unknown outcome, or a write
// that failed after GitHub created the review — is one §8.4.4's adoption has to
// read them back for, and a write placed after the call is the write that may
// not land.
func recordSentRecords(l state.Layout, round *state.Meta, sent *post.Review) error {
	ids := make([]string, 0, len(sent.Comments))
	for i := range sent.Comments {
		ids = append(ids, sent.Comments[i].Record)
	}
	held, err := l.LockPR(round.Owner, round.Repo, round.PR)
	if err != nil {
		return err
	}
	if err := state.UpdateRoundSection(
		held, round.Round, state.FilePosted, postedRecordsSection, ids,
	); err != nil {
		// The lock is released on the way out of every branch, and the
		// write's own failure is what the caller is told about.
		_ = held.Unlock()
		return err
	}
	return held.Unlock()
}

// postedDiscardsSection is the field the draft's discards are written to
// posted.json under, beside the record ids and for their reason: §2.3's table
// gives a round one posted.json.
const postedDiscardsSection = "discards"

// recordSentDiscards writes the records the draft discarded, with the
// disposition each was discarded under, into posted.json beside the payload.
//
// It runs before the call for the reason recordSentRecords does. The waivers
// those discards call for are written only once the call has succeeded, because
// a waiver standing after GitHub refused the review would outlive the decision
// it recorded: the record stays queued, and a disposition the reviewer clears
// before posting again would leave the waiver silencing its class anyway. So a
// call whose outcome cr never learned leaves the waivers owed, and §8.4.4's
// adoption reads them from here.
//
// The section is written on every send, empty or not, so a discard from an
// earlier send of the round is never read as one of this send's.
func recordSentDiscards(l state.Layout, round *state.Meta, discarded []*finding.Finding) error {
	discards := make([]post.Discard, 0, len(discarded))
	for _, record := range discarded {
		discards = append(discards, post.Discard{Record: record.ID, Disposition: record.Disposition})
	}
	held, err := l.LockPR(round.Owner, round.Repo, round.PR)
	if err != nil {
		return err
	}
	if err := state.UpdateRoundSection(
		held, round.Round, state.FilePosted, postedDiscardsSection, discards,
	); err != nil {
		// The lock is released on the way out of every branch, and the
		// write's own failure is what the caller is told about.
		_ = held.Unlock()
		return err
	}
	return held.Unlock()
}

// postedOutcomesSection is the field the draft's §7.3.1 outcomes are written to
// posted.json under, beside the record ids and for their reason: §2.3's table
// gives a round one posted.json.
const postedOutcomesSection = "outcomes"

// recordSentOutcomes writes the outcome the draft's triage settled on for every
// record it read into posted.json, beside the payload.
//
// It runs before the call for the reason recordSentRecords does. The events are
// written only once the call has succeeded, for the reason recordPostTriage
// gives, so a call whose outcome cr never learned leaves them owed — and a
// softening is not a state findings.ndjson holds, while a question in the
// payload may be §6.3.2's forcing rather than the reviewer's, so §8.4.4's
// adoption can read the outcomes from here and from nowhere else.
//
// Each outcome carries the record's register, grade, severity and anchor as
// this run holds it, which is what a successful send stores: §7.2.2's
// recomputation, the forcings and §7.2's two editable rows are applied in
// memory before the payload is built and reach findings.ndjson only after the
// call, so they too survive an unknown outcome only here.
//
// The section is written on every send, empty or not, so an outcome from an
// earlier send of the round is never read as one of this send's.
func recordSentOutcomes(l state.Layout, round *state.Meta, settled []finding.Settled) error {
	outcomes := make([]post.Settlement, 0, len(settled))
	for _, one := range settled {
		outcomes = append(outcomes, post.Settlement{
			Record: one.Record.ID, Outcome: one.Outcome,
			Kind: one.Record.Kind, Grade: one.Record.Grade,
			Severity: one.Record.Severity, Anchor: one.Record.Anchor,
		})
	}
	held, err := l.LockPR(round.Owner, round.Repo, round.PR)
	if err != nil {
		return err
	}
	if err := state.UpdateRoundSection(
		held, round.Round, state.FilePosted, postedOutcomesSection, outcomes,
	); err != nil {
		// The lock is released on the way out of every branch, and the
		// write's own failure is what the caller is told about.
		_ = held.Unlock()
		return err
	}
	return held.Unlock()
}

// writePosted is §8.3.3's first half: the exact payload written to
// rounds/<n>/posted.json before the network call. It answers with the bytes it
// wrote, which are what post.Create sends as the request body.
//
// The bytes and not the file are sent, because the file does not stay the
// payload: recordSentRecords, recordSentDiscards and recordSentOutcomes add cr's
// own sections to it before the call, for §8.4.4's adoption to read, and none of
// them is a field GitHub's review-creation endpoint names. So the request is the
// payload alone — no record id, grade, severity, anchor or disposition leaves
// the machine — while the document on disk holds the payload and those sections
// beside it. The payload hash is not in either: §8.4.3 embeds that in the
// review's body, §10.3 records it in the round summary, and adding it here would
// put a field GitHub never named into the request.
//
// The trailing newline is the one every document cr writes ends with, and it
// is inside what is sent rather than beside it, so the bytes sent are the bytes
// that were written.
func writePosted(
	l state.Layout, owner, repo string, pr, round int, review *post.Review,
) ([]byte, error) {
	payload, err := review.Payload()
	if err != nil {
		return nil, err
	}
	payload = append(payload, '\n')
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return nil, err
	}
	if err := held.WriteRound(round, state.FilePosted, payload); err != nil {
		// The lock is released on the way out of every branch, and the
		// write's own failure is what the caller is told about.
		_ = held.Unlock()
		return nil, err
	}
	if err := held.Unlock(); err != nil {
		return nil, err
	}
	return payload, nil
}

// postedEntries forms §9.3.6's entry for each record, in the order given, and
// writes nothing.
//
// review is the node id of the review the records reached the author in, which
// every entry carries.
//
// Each entry's key reads the record's anchored lines from the trees of the head
// the record was produced against, for the reason discardWaivers gives. It is
// the one place an entry is formed, for `cr post --confirm` and for
// `cr post --reconcile`'s adoption alike, so the two cannot key one record
// differently. `--confirm` forms them before its request, with the review not
// yet created, so a read that fails refuses a post that has sent nothing; an
// adoption forms them once it has matched its review, and a read that fails
// there leaves `post_unresolved` set, so the next `--reconcile` forms them again.
func postedEntries(
	round *state.Meta, records []*finding.Finding, review string,
) ([]finding.PostedEntry, error) {
	entries := make([]finding.PostedEntry, 0, len(records))
	for _, record := range records {
		entry, err := finding.PostedEntryFor(
			keyTrees(round.Owner, round.Repo, round.PR, record.Head), record, review)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// appendPostedIndex is §9.3.6's writer: one entry per record this run moved to
// `posted`, as postedEntries formed them, appended to `posted-index.ndjson`. It
// reads no tree.
//
// `cr post` is the writer, and round 8's unassigned-writer is why that is
// written down rather than inferred. The file is load-bearing in one direction
// only — a missing entry silently re-raises a comment the author already
// received — and §9.1 moves a record to `posted` exactly here, on `--confirm`
// and on `--reconcile`'s adoption alike, so any other command appending would
// be recording a state it did not cause.
//
// It is appended after the network write has returned, beside the thread ids:
// an entry written before the call would suppress next round's finding about a
// comment the author never got.
func appendPostedIndex(l state.Layout, round *state.Meta, entries []finding.PostedEntry) error {
	owner, repo, pr := round.Owner, round.Repo, round.PR
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return err
	}
	if err := finding.AppendPosted(held, l, owner, repo, pr, entries); err != nil {
		// The lock is released on the way out of every branch, and the
		// write's own failure is what the caller is told about.
		_ = held.Unlock()
		return err
	}
	return held.Unlock()
}

// adoptThreads is §8.3.3's second half: posted.json updated with the thread ids
// the network call returned, keyed by the record each comment was drawn from.
//
// The update adds a field and rewrites none, through the same
// UpdateRoundSection §10.3's summary is accumulated with. That is what keeps
// the document the payload afterwards: a writer that decoded posted.json into a
// struct of its own would re-encode it without every field that struct does not
// name, and the payload the round was posted with would be gone.
//
// The key is the record id and not the comment's position. §9.1 moves a record
// to `posted` with the thread it reached the author in, and a position is not a
// name: a later round's payload holds different comments in a different order,
// so an index would point at whatever now stands there.
func adoptThreads(
	l state.Layout, owner, repo string, pr, round int, threads map[string]string,
) error {
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return err
	}
	if err := state.UpdateRoundSection(
		held, round, state.FilePosted, postedThreads, threads,
	); err != nil {
		_ = held.Unlock()
		return err
	}
	return held.Unlock()
}
