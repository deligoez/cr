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

// writePosted is §8.3.3's first half: the exact payload written to
// rounds/<n>/posted.json before the network call. It answers with the path,
// which is what post.Request sends as `--input`.
//
// The file is the request body and not a description of it. post.Request reads
// the body from a file for exactly this reason — the bytes that were written
// are the bytes that are sent, with no second serialisation between them that
// could differ — so nothing of cr's own is mixed into the document before the
// call. The payload hash is not in it: §8.4.3 embeds that in the review's body,
// §10.3 records it in the round summary, and adding it here would put a field
// GitHub never named into the request.
//
// The trailing newline is the one every document cr writes ends with, and it
// is inside what is sent rather than beside it, so the file stays exactly the
// payload rather than the payload plus a note.
func writePosted(
	l state.Layout, owner, repo string, pr, round int, review *post.Review,
) (string, error) {
	payload, err := review.Payload()
	if err != nil {
		return "", err
	}
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return "", err
	}
	if err := held.WriteRound(round, state.FilePosted, append(payload, '\n')); err != nil {
		// The lock is released on the way out of every branch, and the
		// write's own failure is what the caller is told about.
		_ = held.Unlock()
		return "", err
	}
	if err := held.Unlock(); err != nil {
		return "", err
	}
	return l.RoundFile(owner, repo, pr, round, state.FilePosted), nil
}

// recordPostedIndex is §9.3.6's writer: one entry per record this run moved to
// `posted`, appended to `posted-index.ndjson`.
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
func recordPostedIndex(l state.Layout, round *state.Meta, records []*finding.Finding) error {
	owner, repo, pr := round.Owner, round.Repo, round.PR
	entries := make([]finding.PostedEntry, 0, len(records))
	for _, record := range records {
		entries = append(entries, finding.PostedEntryFor(record))
	}
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
