package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/state"
)

// sending is everything §8.3's one call needs about the round it sends.
//
// It is a struct rather than a parameter list because the send has to happen
// exactly once and in one order: §8.4.1 makes the call atomic, so there is no
// partial send to resume from, and a caller assembling six arguments in the
// wrong order is a caller that can post a round's comments against another
// round's state. The gate is deliberately not a field — it arrives at send as
// an argument, so nothing that is merely constructed carries the permission.
type sending struct {
	// layout is the state root every write below goes through.
	layout state.Layout
	// round is the pull request and the round being posted.
	round *state.Meta
	// review is §8.3.1's single review, built and already validated.
	review *post.Review
	// records are the round's records as they were read, which is the
	// slice §9.1's walk writes back: the posted ones move and every other
	// record of the round is republished exactly as it stands.
	records []*finding.Finding
	// queued are the records the payload's comments were drawn from, in
	// payload order, which is what the report names. They are not the
	// slice above: §7.2's softening and hardening reach the payload as
	// copies, so a state walk over these would move a copy and store
	// nothing.
	queued []*finding.Finding
	// forced is §6.3.2's count over them, reported after the send exactly
	// as §8.5.1's dry run reports it.
	forced finding.Forcings
	// withdrawn is §3.6.6's count over them, reported beside it.
	withdrawn finding.Withdrawn
	// triage is what the draft's verbs made of the round, which §7.3.1 has
	// this command settle once the call has returned.
	triage *triaged
	// journal is §9.1.1's record of the moves this run makes, the draft's
	// discards read above included, published with the records they moved.
	journal *finding.Journal
}

// send performs §8.3's network write and everything §8.4, §9.1, §9.3.6 and
// §7.3.1 owe the round once it has returned.
//
// The order is the sections' and not a convenience.
//
// §8.3.3 puts the exact payload on disk before the call, and the call sends the
// bytes that write produced: post.Create hands them to gh on standard input, so
// the bytes written are the bytes sent, and the sections cr adds to posted.json
// next are never part of the request. The file is also what §8.4.4's recovery
// reads back, and a payload written afterwards would be missing from exactly
// the run that could not say whether it had posted.
//
// Everything after the call is ordered by what it costs to lose. §9.1's walk
// and §9.3.6's index go first: a review that reached the author while cr still
// holds its records in `queued` is the failure the posted index exists to
// prevent, and the next round would re-raise every comment the author has
// already read. §7.3.1's outcome events follow, because they describe a review
// that now exists — round 8's triage-event-key-permits-contradiction is the
// other half of that, and it is why nothing here runs on the rejection branch.
// §8.3.3's thread ids come last: they are provenance for a v0.2 that migrates
// anchors, and losing them costs a reader one lookup rather than costing the
// author a duplicate comment.
//
// §7.4's waivers for the draft's discards are written once the call has
// succeeded and ahead of that walk, through the waiveDiscards `cr draft` calls,
// so a block deleted or marked `wrong` and then posted without a redraft is
// waived exactly as a redraft would have waived it. They are not written before
// the call: a review GitHub refuses leaves the discarded records queued, and a
// waiver already standing would outlive a disposition the reviewer then clears
// — a `wrong` silencing its class across the repository for a record that was
// posted after all. They go ahead of the walk, which stores the discards,
// because a discard stored with its waiver lost is never read again. The
// discards are named in posted.json before the call, so a call whose outcome cr
// never learned, or a waiver write that fails after the call while
// `post_unresolved` is still set, leaves them for §8.4.4's adoption to waive.
//
// `post_unresolved` is set before the call and cleared only once the records
// are marked posted, so the refusal send opens with rests on a write that
// landed before anything was sent. A write after a successful call can fail —
// the disk, the lock, a process killed mid-call — and a flag that only such a
// write could set would leave the round looking unsent, which §8.4.4 calls the
// worse failure.
func (s *sending) send(out *writer, confirmation gh.Confirmation) error {
	owner, repo, pr := s.round.Owner, s.round.Repo, s.round.PR
	if s.round.PostUnresolved {
		return &UnresolvedPostError{Owner: owner, Repo: repo, PR: pr, Round: s.round.Round}
	}
	payload, err := writePosted(s.layout, owner, repo, pr, s.round.Round, s.review)
	if err != nil {
		return err
	}
	if err := recordSentRecords(s.layout, s.round, s.review); err != nil {
		return err
	}
	if err := recordSentDiscards(s.layout, s.round, s.triage.discarded()); err != nil {
		return err
	}
	if err := recordSentOutcomes(s.layout, s.round, s.triage.settled()); err != nil {
		return err
	}
	if err := setPostUnresolved(s.layout, s.round, true); err != nil {
		return err
	}
	if _, err := post.Create(confirmation, owner, repo, pr, payload); err != nil {
		// §8.4: GitHub's refusal is §8.4.2 and everything else is
		// §8.4.4's unknown outcome. Neither marks anything posted nor
		// waives anything, so every write below is on this function's
		// other branch.
		return postOutcome(s.layout, s.round, s.review, err)
	}
	if err := waiveDiscards(s.layout, owner, repo, pr, s.triage.discarded()); err != nil {
		return fmt.Errorf(
			"the review was created and the draft's waivers could not be written for round %d; "+
				"post_unresolved stays set, so nothing can be sent twice, and `cr post %d --reconcile` "+
				"adopts the review and writes them: %w",
			s.round.Round, pr, err,
		)
	}
	if err := s.markPosted(); err != nil {
		return err
	}
	if err := setPostUnresolved(s.layout, s.round, false); err != nil {
		return err
	}
	if err := recordPostTriage(s.layout, owner, repo, pr, s.round, s.triage.settled()); err != nil {
		return err
	}
	if err := adoptReturnedThreads(s.layout, s.round, s.records, s.review); err != nil {
		return err
	}
	return out.emit(&postResult{
		Round: s.round.Round, Comments: commentedRecords(s.review, s.queued),
		Payload: s.review, Forced: s.forced, Withdrawn: s.withdrawn,
		posting: posting{Posted: true, ConfirmGiven: true},
	})
}

// markPosted walks §9.1's `queued` → `posted` row over the records the payload
// carried, and writes §9.3.6's index entries for them.
//
// The records are the payload's and not the round's, for the reason
// adoptAsPosted gives about the same walk: a record the reviewer discarded in
// the draft never reached the comments, so moving the round wholesale would
// mark as sent things nobody received. It shares writeAdopted with that path
// because the two are one operation — §9.1 lists `cr post --confirm` and
// `cr post --reconcile` on the same row — and two writers of `posted` that
// could disagree about the index is exactly what §9.3.6 cannot survive.
//
// A record already in `posted` is passed over rather than moved again. §9.1
// lists no move out of `posted`, so asking for one would refuse the whole walk
// on the strength of an earlier run having worked.
func (s *sending) markPosted() error {
	sent := make([]*finding.Finding, 0, len(s.review.Comments))
	for i := range s.review.Comments {
		record := recordOf(s.records, s.review.Comments[i].Record)
		if record == nil || record.State == finding.StatePosted {
			continue
		}
		if err := s.journal.Move(
			record.ID, finding.Existing(record.State), finding.StatePosted,
		); err != nil {
			return err
		}
		record.State = finding.StatePosted
		sent = append(sent, record)
	}
	// §8.3.3's hash of the payload that was just sent, which §10.3 records
	// in the round summary beside the posted count.
	hash, err := s.review.Hash()
	if err != nil {
		return err
	}
	return writeAdopted(s.layout, s.round, s.records, sent, hash, s.journal)
}

// adoptReturnedThreads is §8.3.3's second half: posted.json updated with the
// thread ids the call produced, keyed by the record each comment came from.
//
// They are read back rather than taken out of the response, because the
// response does not carry them. GitHub's review-creation call answers with the
// review — its id, its body, its state — and names no thread per comment, so
// the only place the ids exist is the pull request's review threads, which
// internal/gh already reads whole for §3.5.1's ingestion.
//
// The match is on the body, which is the one field cr wrote and GitHub echoes
// unchanged. An anchor would not settle it: §6.4.1 groups by path, side, line
// and class, so two comments of one round can share every part of a position
// and differ only in what they say — and a match by position would then key one
// thread id onto the wrong record. Payload order would be worse still, being
// GitHub's ordering rather than cr's.
//
// A comment no thread matches is left out rather than guessed at, and the ids
// that were found are still recorded: §8.3.3 asks for the returned ids, and a
// map short by one is a truer answer than a map holding an id for a comment cr
// could not identify.
//
// `cr post --reconcile` calls it too, once it has adopted a review as posted:
// §8.3.3 asks for the ids of the review the round became, and a review adopted
// after an unknown outcome is that review exactly as a returned one is. Both
// callers read through this one function, so the two paths cannot key a
// record's thread differently. It performs a read and nothing else on the
// network.
func adoptReturnedThreads(
	l state.Layout, round *state.Meta, records []*finding.Finding, review *post.Review,
) error {
	threads, err := ghClient().Threads(round.Owner, round.Repo, round.PR)
	if err != nil {
		return fmt.Errorf(
			"the review was created and §8.3.3's thread ids could not be read back "+
				"for round %d; nothing needs re-sending, and `cr status %d` reports the "+
				"records as posted: %w",
			round.Round, round.PR, err,
		)
	}
	found := threadsByRecord(threads, review)
	if err := adoptThreads(l, round.Owner, round.Repo, round.PR, round.Round, found); err != nil {
		return err
	}
	return storeThreadIDs(l, round, records, found)
}

// storeThreadIDs writes §6.1's `thread_id` onto each posted record the returned
// ids name, and republishes the round's records with them.
//
// It is the same map posted.json was just given, so the two cannot name
// different threads for one record. It runs after `post_unresolved` is cleared
// and the records are marked posted, so a write that fails here costs the field
// and never reopens the round to a second send.
func storeThreadIDs(
	l state.Layout, round *state.Meta, records []*finding.Finding, found map[string]string,
) error {
	for _, record := range records {
		if id, matched := found[record.ID]; matched {
			record.ThreadID = id
		}
	}
	held, err := l.LockPR(round.Owner, round.Repo, round.PR)
	if err != nil {
		return err
	}
	stamp := state.Stamp{Head: round.Head, Round: round.Round}
	if err := state.ReplaceStamped(held, state.FileFindings, stamp, records); err != nil {
		// The lock is released on the way out of every branch, and the
		// write's own failure is what the caller is told about.
		_ = held.Unlock()
		return err
	}
	return held.Unlock()
}

// threadsByRecord is the thread each of the payload's comments became, keyed by
// the record it was drawn from.
//
// A thread is claimed once. Two comments carrying the same body would otherwise
// both name the first thread that matched, which would record one id twice and
// none for the second — and the pairing is what the id is for.
func threadsByRecord(threads []gh.Thread, review *post.Review) map[string]string {
	byBody := make(map[string]string, len(threads))
	for i := range threads {
		if _, claimed := byBody[threads[i].Comment.Body]; !claimed {
			byBody[threads[i].Comment.Body] = threads[i].ID
		}
	}
	found := make(map[string]string, len(review.Comments))
	for i := range review.Comments {
		comment := &review.Comments[i]
		if id, matched := byBody[comment.Body]; matched {
			found[comment.Record] = id
			delete(byBody, comment.Body)
		}
	}
	return found
}
