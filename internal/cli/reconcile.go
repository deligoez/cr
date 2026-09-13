package cli

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/state"
)

// reconcileResult is what `cr post --reconcile` reports: the round it looked
// at, the hash it matched on, the review it found, and where `post_unresolved`
// stands once the run is over.
//
// It reports the hash whether or not a review carried it, because that value is
// what a reviewer takes to the pull request when cr found nothing: §8.4.3
// embeds it in the review body, so a human can search for it and settle by hand
// the question this run could not.
type reconcileResult struct {
	// Round is the round whose payload was matched.
	Round int `json:"round"`
	// PayloadHash is §8.4.3's hash of that payload, empty when there was
	// no posting to reconcile.
	PayloadHash string `json:"payload_hash"`
	// Adopted is the url of the review that carried the hash, and empty
	// when no review did.
	Adopted string `json:"adopted"`
	// Records are the records the adoption moved to `posted`, in payload
	// order. It is empty and never nil.
	//
	// It is not §12.6's `posted` field and must not be spelled as one: that
	// field says whether this run performed the network write, and this
	// run performs none. What it reports is the state cr brought into
	// agreement with a review that was already there.
	Records []string `json:"records"`
	// Unresolved is `post_unresolved` as this run leaves it on meta.json.
	Unresolved bool `json:"post_unresolved"`
	// Honesty carries §9.3.1's comparison of the round's head against the
	// pull request's current one. §9.3.2 exempts this run from the
	// refusal and exempts it from nothing else, so the report is still
	// owed.
	Honesty []string `json:"honesty"`
}

// Text says what the run settled, then the disclosure.
func (r *reconcileResult) Text(w *writer) string {
	text := ""
	switch {
	case r.PayloadHash == "":
		text = "round " + strconv.Itoa(r.Round) + " has no unresolved posting to reconcile\n"
	case r.Adopted == "":
		text = "no review carries payload hash " + w.accent(r.PayloadHash) +
			", so post_unresolved is cleared and the round may be posted again\n"
	default:
		text = "adopted " + w.accent(r.Adopted) + " as round " + strconv.Itoa(r.Round) +
			"'s review: " + strconv.Itoa(len(r.Records)) + " record(s) are posted\n"
	}
	return text + w.disclose("", "\n", r.Honesty...)
}

// reconcilePost is §8.4.4's recovery: the pull request's reviews are listed,
// §8.4.3's embedded payload hash is matched, and the round is either adopted as
// posted or released for a retry.
//
// It runs only when `post_unresolved` is set. §8.4.4 is about an outcome cr
// never learned, and a round with no such outcome has nothing to reconcile —
// while adopting a review for one would move records to `posted` on the
// strength of a hash match nobody asked about.
//
// Nothing is posted here and nothing is retried. §8.5.2 makes `--confirm` the
// whole of the network-write gate and this run does not carry it: what it does
// is read the pull request and write cr's own state to agree with what it
// found. That is the whole of "posting twice is a worse failure than posting
// late" — the recovery from not knowing is to look, never to send again.
func reconcilePost(out *writer, l state.Layout, round *state.Round) error {
	result := &reconcileResult{
		Round: round.Round, Unresolved: round.PostUnresolved,
		Records: make([]string, 0), Honesty: []string{round.Disclosure()},
	}
	if !round.PostUnresolved {
		return out.emit(result)
	}
	sent, err := postedPayload(l, &round.Meta)
	if err != nil {
		return err
	}
	if result.PayloadHash, err = sent.Hash(); err != nil {
		return err
	}
	if result.Adopted, err = reviewCarrying(&round.Meta, result.PayloadHash); err != nil {
		return err
	}
	if result.Adopted == "" {
		if err := setPostUnresolved(l, &round.Meta, false); err != nil {
			return err
		}
		result.Unresolved = false
		return out.emit(result)
	}
	if result.Records, err = adoptAsPosted(l, &round.Meta, sent); err != nil {
		return err
	}
	result.Unresolved = false
	return out.emit(result)
}

// postedPayload reads back the payload §8.3.3 wrote before the call.
//
// The hash is taken from that file rather than from a fresh build of the round.
// §8.4.3 embedded the hash of what was sent, and a round rebuilt now would
// rebuild it out of whatever the draft and the records hold at this moment — so
// a body edited since, or a record triaged since, would produce a hash no
// review can carry and a reconciliation that always answers "not posted".
//
// A round whose posted.json holds no comment is refused rather than matched.
// The hash of an empty payload is a real digest and would match any other empty
// round's, and answering "no review carries it" would then clear
// `post_unresolved` for a round that may well have posted — which is the double
// post §8.4.4 calls the worse failure.
func postedPayload(l state.Layout, round *state.Meta) (*post.Sent, error) {
	body, err := l.ReadRound(round.Owner, round.Repo, round.PR, round.Round, state.FilePosted)
	if err != nil {
		return nil, err
	}
	at := l.RoundFile(round.Owner, round.Repo, round.PR, round.Round, state.FilePosted)
	sent, err := post.Decode(body)
	if err != nil {
		return nil, state.FileFailure("read", at, state.UnusableHint, err)
	}
	// The three refusals below are one answer: posted.json is cr's own
	// file, it was found and parsed, and it cannot be used for §8.4.4's
	// match. §11.2 codes that 3, as it codes state.ContextStoreError — the
	// command line is right, so code 2 would tell the reader to retype it,
	// and the payload is not the agent's input, so code 1 would name the
	// wrong author.
	if len(sent.Comments) == 0 {
		return nil, state.FileFailure("use", at, state.UnusableHint, fmt.Errorf(
			"round %d carries post_unresolved and the payload holds no comment: "+
				"§8.3.3 writes the file before the call, so there is no §8.4.3 hash "+
				"to match and cr will not guess whether the review was created",
			round.Round,
		))
	}
	if len(sent.Records) != len(sent.Comments) {
		return nil, state.FileFailure("use", at, state.UnusableHint, fmt.Errorf(
			"the payload holds %d comment(s) and names %d record(s): the run that "+
				"could not establish its outcome writes both, so cr cannot say which "+
				"records reached the author and will not mark any of them posted",
			len(sent.Comments), len(sent.Records),
		))
	}
	// Comment.Record is not in the document, so a decoded payload names no
	// record until the ids written beside it are paired back, by position:
	// they are in payload order and the counts were just held equal. The
	// hash needs them, because §8.3.3 orders two comments at one position by
	// record id, and a pre-image built without ids would order them as the
	// payload does and miss the hash the send embedded.
	for i := range sent.Comments {
		sent.Comments[i].Record = sent.Records[i]
	}
	return sent, nil
}

// reviewCarrying is §8.4.4's match: the url of the pull request's review whose
// body embeds hash, and the empty string when none does.
//
// The first match wins and the walk stops there. Two reviews carrying one hash
// would mean the round was posted twice already, which nothing this run does
// can undo; adopting the earlier one is the answer that names the review the
// author read first.
func reviewCarrying(round *state.Meta, hash string) (string, error) {
	reviews, err := ghClient().Reviews(round.Owner, round.Repo, round.PR)
	if err != nil {
		return "", err
	}
	for _, review := range reviews {
		if embedded, found := render.PayloadHashIn(review.Body); found && embedded == hash {
			return review.URL, nil
		}
	}
	return "", nil
}

// adoptAsPosted walks §9.1's `queued` → `posted` row over the records the
// payload carried, appends §9.3.6's index entries for them, clears
// `post_unresolved`, and then does what `cr post --confirm` does once its call
// has returned: the draft's discards are waived, §7.3.1's outcome events are
// written, and §8.3.3's thread ids are read back and stored.
//
// The records are the payload's and not the round's. A record the reviewer
// discarded in the draft never reached the comments, and a record recorded
// since the call was made was never in front of the author — so adopting the
// round wholesale would mark as sent things nobody received.
//
// A record already in `posted` is counted and not moved again, which is what
// makes a second `--reconcile` harmless: §9.1 lists no move out of `posted`, so
// asking for one would refuse the whole adoption on the strength of the first
// one having worked.
//
// Before anything is written, the records the send read are given the values it
// held them at, as posted.json's outcomes name them, so every write below —
// the waivers, findings.ndjson through writeAdopted, and the outcome events —
// draws on the records a successful send would have drawn on.
//
// The draft's discards are stored with the adopted records, and their waivers
// go first, in send's order and for its reason: the send that set
// `post_unresolved` wrote none, because a call GitHub might have refused must
// leave none behind, and a waiver write that fails here leaves the flag set so
// the next `--reconcile` writes it again. The discard is the confirmed send's
// own move and is journaled under its actor, which §9.1's `queued` →
// `discarded` row lists: that run read the draft, discarded the records, and
// named them in posted.json before its call, and this adoption settles that
// run rather than triaging a draft of its own. Storing it is what keeps a
// waiver from standing for a record left queued, which a later draft could
// otherwise clear and a later send post under it.
//
// The outcome events follow the cleared flag, in send's order, through the
// recordPostTriage the send calls, over the outcomes the send named in
// posted.json before its call.
//
// The thread ids go last, through the adoptReturnedThreads the send uses, once
// the flag is cleared — a read that fails there costs the field and never
// reopens the round to a second send. It is a read: nothing here posts.
func adoptAsPosted(l state.Layout, round *state.Meta, sent *post.Sent) ([]string, error) {
	records, err := roundFindingsOf(l, round.Owner, round.Repo, round.PR, round.Round)
	if err != nil {
		return nil, err
	}
	settleAsSent(records, sent)
	sendJournal := finding.NewJournal(finding.ActorPostConfirm, round.Head, time.Now())
	discarded, err := discardAsSent(records, sent, sendJournal)
	if err != nil {
		return nil, err
	}
	if err := waiveDiscards(l, round.Owner, round.Repo, round.PR, discarded); err != nil {
		return nil, err
	}
	adopted := make([]*finding.Finding, 0, len(sent.Records))
	ids := make([]string, 0, len(sent.Records))
	journal := finding.NewJournal(finding.ActorPostReconcile, round.Head, time.Now())
	for _, sentID := range sent.Records {
		record := recordOf(records, sentID)
		if record == nil || record.State == finding.StatePosted {
			continue
		}
		if err := journal.Move(
			record.ID, finding.Existing(record.State), finding.StatePosted,
		); err != nil {
			return nil, err
		}
		record.State = finding.StatePosted
		adopted = append(adopted, record)
		ids = append(ids, record.ID)
	}
	hash, err := sent.Hash()
	if err != nil {
		return nil, err
	}
	if err := writeAdopted(l, round, records, adopted, hash, sendJournal, journal); err != nil {
		return nil, err
	}
	if err := setPostUnresolved(l, round, false); err != nil {
		return nil, err
	}
	if err := recordPostTriage(
		l, round.Owner, round.Repo, round.PR, round, sentSettlements(records, sent),
	); err != nil {
		return nil, err
	}
	return ids, adoptReturnedThreads(l, round, records, &sent.Review)
}

// settleAsSent writes onto each record posted.json's outcomes name the
// register, grade, severity and anchor the send held it at when it built the
// payload: §7.2.2's recomputed grade, the forcings, and §7.2's severity and
// location rows, which a successful send stores and an unknown outcome left in
// memory.
//
// Every named record takes them, whatever state it now holds, for the reason
// sentSettlements gives: they are the send's reading of its draft, and a
// successful send stores them on every record it read. A record the round no
// longer holds is passed over.
func settleAsSent(records []*finding.Finding, sent *post.Sent) {
	for i := range sent.Outcomes {
		outcome := &sent.Outcomes[i]
		if record := recordOf(records, outcome.Record); record != nil {
			record.Kind, record.Grade = outcome.Kind, outcome.Grade
			record.Severity, record.Anchor = outcome.Severity, outcome.Anchor
		}
	}
}

// sentSettlements are the outcomes posted.json names for the send's triage,
// each paired with the round's record it names, which is what recordPostTriage
// writes §7.3.1's events from.
//
// Every named record is settled, whatever state it now holds: the outcome is
// the send's reading of its draft, and the event is keyed by pull request,
// round and record, so a discard a later `cr draft` already settled is written
// again under its own key rather than beside it. A record the round no longer
// holds is passed over, since there is nothing to draw an event's fields from.
func sentSettlements(records []*finding.Finding, sent *post.Sent) []finding.Settled {
	settled := make([]finding.Settled, 0, len(sent.Outcomes))
	for i := range sent.Outcomes {
		outcome := &sent.Outcomes[i]
		if record := recordOf(records, outcome.Record); record != nil {
			settled = append(settled, finding.Settled{Record: record, Outcome: outcome.Outcome})
		}
	}
	return settled
}

// discardAsSent moves each record posted.json names as the draft's discards to
// `discarded` under the disposition it was discarded under, keeping the move in
// the confirmed send's journal, and answers the records it moved, which is what
// waiveDiscards derives a waiver's scope from.
//
// A record no longer queued is passed over: a later `cr draft` already stored
// its discard and wrote its waiver, and a record posted since was not discarded
// at all.
func discardAsSent(
	records []*finding.Finding, sent *post.Sent, journal *finding.Journal,
) ([]*finding.Finding, error) {
	discarded := make([]*finding.Finding, 0, len(sent.Discards))
	for _, discard := range sent.Discards {
		record := recordOf(records, discard.Record)
		if record == nil || record.State != finding.StateQueued {
			continue
		}
		if err := journal.Move(
			record.ID, finding.Existing(finding.StateQueued), finding.StateDiscarded,
		); err != nil {
			return nil, err
		}
		record.State, record.Disposition = finding.StateDiscarded, discard.Disposition
		discarded = append(discarded, record)
	}
	return discarded, nil
}

// recordOf is the round's record with one id, and nil when the round holds
// none.
func recordOf(records []*finding.Finding, id string) *finding.Finding {
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	return nil
}

// writeAdopted publishes the round's records with the adopted ones in `posted`,
// the §9.3.6 entries for them, and `cr post`'s share of §10.3's round summary:
// how many of the round's records are posted, hash, the §8.3.3 payload hash of
// the review they reached the author in, and §8.5.4's fact that `--confirm`
// was given.
//
// That fact holds on the `--reconcile` path as well, and not by assumption.
// reconcilePost adopts only while `post_unresolved` is set, and send and
// postOutcome are its only setters, reached only from a send — which takes the
// confirmation the gate minted. So a review adopted here is one a confirmed run
// sent. Nothing else about the gate is recorded, because §8.5.4 allows nothing
// more.
//
// The index is written from the records this run moved rather than from every
// record in `posted`: a round reconciled twice would otherwise re-append what
// the first run already recorded, and AppendPosted's key check is a second
// guard rather than the reason this one holds.
//
// The summary is finalised here because this is the one place §9.1's `posted`
// is written, by `cr post --confirm` and `cr post --reconcile` alike. A round
// that never reaches it — drafted and never sent, or run only through §8.5.1's
// dry run — keeps a summary without `cr post`'s counts, which is a truer record
// of that round than a count of zero beside a hash nothing was sent under.
//
// The two discard counts are rewritten beside them, over the records stored
// here: a confirmed send stores the draft's discards itself, and a round posted
// with no redraft after a deletion would otherwise keep the last `cr draft`'s
// counts beside records that say otherwise.
//
// The journals are written in the order given, under the lock the records are
// published under: an adoption passes the confirmed send's discards ahead of
// its own posted moves, which is the order that send's one journal holds them.
func writeAdopted(
	l state.Layout, round *state.Meta, records, adopted []*finding.Finding, hash string,
	journals ...*finding.Journal,
) error {
	if err := recordPostedIndex(l, round, adopted); err != nil {
		return err
	}
	held, err := l.LockPR(round.Owner, round.Repo, round.PR)
	if err != nil {
		return err
	}
	stamp := state.Stamp{Head: round.Head, Round: round.Round}
	posted := 0
	for _, record := range records {
		if record.State == finding.StatePosted {
			posted++
		}
	}
	writes := make([]func() error, 0, len(journals)+3)
	for _, journal := range journals {
		writes = append(writes, func() error { return journal.Write(held) })
	}
	writes = append(writes,
		func() error { return state.ReplaceStamped(held, state.FileFindings, stamp, records) },
		func() error {
			return writeSummary(held, round.Round, ownerPost, []summaryCount{
				{key: summaryPosted, value: posted},
				{key: summaryPayloadHash, value: hash},
				{key: summaryConfirmGiven, value: true},
			})
		},
		func() error { return writeSummary(held, round.Round, ownerDiscards, discardCounts(records)) },
	)
	for _, write := range writes {
		if err := write(); err != nil {
			// The lock is released on the way out of every branch, and
			// the write's own failure is what the caller is told about.
			_ = held.Unlock()
			return err
		}
	}
	return held.Unlock()
}

// setPostUnresolved writes `post_unresolved` onto meta.json and leaves every
// other field as the file holds it once the §2.3.1 lock is taken. The round's
// own copy is not what is written back: it was read before the lock, and a `cr
// brief` that took the lock in between would have its fields reverted.
//
// It is the second writer of that file, and §8.4.4 is what makes it one: §3.7
// gives `cr brief` meta.json, and this field is the one thing in it that a
// posting decides. `cr brief` carries the flag forward untouched for the same
// reason — §3.7 decides nothing about it.
//
// The caller's copy is updated as well, so a run that goes on to report the
// flag reports what is on disk rather than what was there when it started.
func setPostUnresolved(l state.Layout, round *state.Meta, unresolved bool) error {
	held, err := l.LockPR(round.Owner, round.Repo, round.PR)
	if err != nil {
		return err
	}
	if err := held.SetPostUnresolved(unresolved); err != nil {
		_ = held.Unlock()
		return err
	}
	if err := held.Unlock(); err != nil {
		return err
	}
	round.PostUnresolved = unresolved
	return nil
}

// postOutcome is §8.4's fork, read off what came back from §8.3's call.
//
// GitHub saying no is §8.4.2, and post.Rejection reads its error document off
// gh.CommandError's standard output — where a failed `gh api` writes it — beside
// the HTTP status gh names on standard error. That refusal is returned with
// `post_unresolved` cleared again: send set it before the call, §8.4.2 marks
// nothing posted, and a rejection is an outcome cr did learn, so the round is
// left postable.
//
// Everything else is §8.4.4's unknown outcome — a timeout, a server error, a
// dropped connection, or a response cr cannot parse — and post.Rejection
// answers nil for exactly those, because reporting "rejected" would be cr
// asserting the one thing it failed to establish. `post_unresolved` is then
// set on meta.json and the caller is told what to run; nothing is retried, and
// there is no branch here that could retry, because a second send is the
// double post §8.4.4 calls the worse failure.
func postOutcome(l state.Layout, round *state.Meta, sent *post.Review, failed error) error {
	if rejected := rejectedPost(sent, failed); rejected != nil {
		return errors.Join(rejected, setPostUnresolved(l, round, false))
	}
	return errors.Join(&UnknownOutcomeError{PR: round.PR, Err: failed},
		recordSentRecords(l, round, sent), setPostUnresolved(l, round, true))
}

// UnresolvedPostError is §8.4.4's refusal of a second send: the round carries
// `post_unresolved`, so a payload §8.3.3 wrote before the call may already have
// become a review, and cr has not learned whether it did.
//
// §11.2 codes it 4. The command line is right and nothing it named is
// malformed; what refuses is where the round stands, and the only way forward
// is `cr post --reconcile`, which adopts the review the call created or clears
// the flag for a retry.
type UnresolvedPostError struct {
	// Owner, Repo, and PR name the pull request whose round was refused.
	Owner string
	Repo  string
	PR    int
	// Round is the round whose posting was never settled.
	Round int
}

func (e *UnresolvedPostError) Error() string {
	return fmt.Sprintf(
		"%s/%s#%d round %d carries post_unresolved: its payload was written before a "+
			"review-creation call whose outcome cr never recorded, and §8.4.4 refuses a "+
			"second send that could post the review twice; run `cr post %d --reconcile "+
			"--repo %s/%s`",
		e.Owner, e.Repo, e.PR, e.Round, e.PR, e.Owner, e.Repo,
	)
}

// UnknownOutcomeError is §8.4.4's report of a review-creation call whose outcome
// cr could not establish: a timeout, a server error, a dropped connection, or a
// response it cannot parse. The review may exist, so `post_unresolved` is set
// and nothing is retried.
//
// §11.2 codes it 4, its partial post. It is deliberately not the 3 the
// gh.CommandError it carries would take: gh ran, the command line is right, and
// what is unsettled is where the round stands, which `cr post --reconcile`
// settles and no fix to the tool would.
type UnknownOutcomeError struct {
	// PR is the pull request whose review-creation call went unanswered.
	PR int
	// Err is the failure the call came back with, so gh's own words and
	// the status it named reach the reviewer.
	Err error
}

func (e *UnknownOutcomeError) Error() string {
	return fmt.Sprintf(
		"§8.4.4: the outcome of the review-creation call is unknown, so post_unresolved is set "+
			"on meta.json and nothing is retried; run `cr post %d --reconcile` to match "+
			"§8.4.3's payload hash against the pull request's reviews: %v",
		e.PR, e.Err,
	)
}

// Unwrap exposes the failure underneath, so the gh command that ran can still
// be read off the error.
func (e *UnknownOutcomeError) Unwrap() error { return e.Err }
