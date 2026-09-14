package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/state"
)

// postResult is what `cr post` reports about the review it validated and built:
// which round it was built from, which records it holds, and §6.3.2's forcing
// count over them.
//
// It carries §12.6's `posted` field because §8.5.2 makes this the one command
// that could have performed the network write: it is the only site that mints a
// confirmation, so it is the only result for which `"posted": false` reports a
// decision something actually made. §8.5.1 requires that value of the dry run in
// as many words, and §8.5.4 bounds what the field may mean — it records that cr
// sent the review, never that a human read the draft.
type postResult struct {
	// Round is the round the review was built from.
	Round int `json:"round"`
	// Comments are the comments the payload carries, in the order §8.3.1's
	// single review holds them.
	Comments []postedComment `json:"comments"`
	// Payload is §8.5.1's full payload: the review as the call would send
	// it, body and comments alike.
	//
	// It is the document and not a summary of it, because §8.5.1 asks for
	// the payload and a reviewer about to give `--confirm` is deciding
	// about exactly these bytes. It is also the only way §8.4.3's review
	// body reaches a reader at all — it is composed at build time and
	// nothing else prints it, so a run without this field would validate a
	// disclosure the author will receive and show it to nobody.
	Payload *post.Review `json:"payload"`
	// Discarded are the ids of the records the draft's triage discards on
	// this run, which a confirmed run stores. It is empty and never nil.
	//
	// It is how a run that builds no review says why: when the draft
	// discards every queued record there is no payload, and this is what
	// the confirmed run settled instead of sending one.
	Discarded []string `json:"discarded"`
	// Forced is §6.3.2's count per class over the records the payload
	// holds. It is reported here as well as by `cr draft` because this is
	// §6.3.1's last application — the only one that sees the payload the
	// author will actually receive.
	Forced finding.Forcings `json:"forced_to_question"`
	// Withdrawn is §3.6.6's count per class of the records the payload
	// holds as questions because the note their claim rests on no longer
	// stands, as this run read the context store.
	Withdrawn finding.Withdrawn `json:"forced_by_retraction"`
	// Warnings are §8.2.3's, one per comment whose suggestion, as the
	// payload carries it, is indented unlike the line it replaces. They
	// are warnings and not refusals: the payload still carries the
	// suggestion as written.
	Warnings []string `json:"warnings"`
	// Honesty is what a reviewer about to give `--confirm` is owed before
	// the payload: that the round carries §8.4.4's `post_unresolved`, so a
	// review of this payload may already exist and no send happens until
	// `cr post --reconcile` has run. It is empty and never nil.
	Honesty []string `json:"honesty"`
	// posting is §12.6's field, which §8.5.1 requires of the dry run in as
	// many words: a run that sent nothing reports `"posted": false`.
	posting
}

// postedComment is one comment of the payload: the record it was drawn from,
// and the register it would reach the author in.
//
// The register is reported because it is the one thing about a comment that can
// still move between the draft the reviewer approved and the payload. §7.2.2
// recomputes the grade here and §6.3 and §4.1.4 are re-applied over the result,
// so a block that read `kind=finding` in the draft can leave as a question —
// and a reviewer who is about to spend their standing on it is owed that before
// it is sent rather than after.
type postedComment struct {
	// ID is the record's id.
	ID string `json:"id"`
	// Kind is the register, after every forcing this run applied.
	Kind finding.Kind `json:"kind"`
}

// Text names the count and the round, then each comment's register, then
// §6.3.2's forcing count.
//
// The forcing line is printed whatever the flags say, for the reason `cr draft`
// prints it: §11.1 exempts it from `--quiet`, and it is how the reader tells a
// review whose questions cr forced from one whose questions the agent chose.
//
// A run that built no review, because the draft discards every queued record,
// says so in place of the count and names each discard.
func (r *postResult) Text(w *writer) string {
	var text strings.Builder
	text.WriteString(w.disclose("", "\n", r.Honesty...))
	if r.Payload == nil {
		text.WriteString("built no review for round " + strconv.Itoa(r.Round) +
			": the draft discards every queued record, so there is no comment to post\n")
		for _, id := range r.Discarded {
			text.WriteString(id + ": discarded\n")
		}
		return text.String() + w.disclose("", "\n", r.Forced.Disclosure(), r.Withdrawn.Disclosure()) + r.line(w)
	}
	text.WriteString("built a review of " + w.accent(strconv.Itoa(len(r.Comments))) +
		" comment(s) for round " + strconv.Itoa(r.Round) + "\n")
	for _, comment := range r.Comments {
		text.WriteString(comment.ID + ": " + string(comment.Kind) + "\n")
	}
	text.WriteString(r.payload())
	for _, warning := range r.Warnings {
		text.WriteString(warning + "\n")
	}
	return text.String() + w.disclose("", "\n", r.Forced.Disclosure(), r.Withdrawn.Disclosure()) + r.line(w)
}

// payload renders §8.5.1's full payload for a terminal: the review's own body,
// then every comment with the record it was drawn from and the position it
// would land at.
//
// It prints the bodies whole and truncates nothing. §8.5.1 asks for the full
// payload, and the reviewer reading it is deciding whether to send these words
// to a colleague — a rendering that elided them would be showing a summary of
// the thing the decision is about.
func (r *postResult) payload() string {
	if r.Payload == nil {
		return ""
	}
	var text strings.Builder
	text.WriteString("\n" + r.Payload.Body + "\n")
	for i := range r.Payload.Comments {
		comment := &r.Payload.Comments[i]
		text.WriteString("\n" + comment.Record + " " + comment.Path + ":" +
			post.Lines(comment.StartLine, comment.Line) + " " + string(comment.Side) + "\n" + comment.Body + "\n")
	}
	return text.String() + "\n"
}

// newPostCmd registers §11's `cr post <pr> [--confirm] [--reconcile]`, the
// validation of §8 and §7.2.2's recomputation before the payload is built.
//
// The two flags are not a pair of options. §8.5.2 makes `--confirm` the whole
// of the gate — without it the run validates, builds a payload, and writes
// nothing — and §8.5.3 forbids any setting, variable, field, or alias that
// supplies it implicitly, which is why it is registered here as a flag and
// nowhere else as anything. `--reconcile` is §8.4.4's recovery from an unknown
// outcome: it lists the pull request's reviews, matches §8.4.3's embedded
// payload hash, and either adopts that review as posted or clears
// `post_unresolved` for a retry. §9.3.2 exempts it from the stale-head refusal,
// because it anchors nothing.
//
// This is the gate's own file, and nowrite_test.go's mintSites names it: §8.5.3
// allows one mint and the flag is its only input, so the token comes into being
// where the flag is read and nowhere else.
func newPostCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "post " + prPlaceholder,
		Short: "Validate and post the review",
		Args:  prArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// §8.5.3: the flag is the whole of the gate's input, read
			// here once. It is not acted on until the payload has
			// been built, which is §8.5.1's ordering.
			confirmed, err := cmd.Flags().GetBool("confirm")
			if err != nil {
				return err
			}
			reconcile, err := cmd.Flags().GetBool("reconcile")
			if err != nil {
				return err
			}
			// --reconcile sends nothing, so a --confirm beside it would
			// be a permission the run silently does not use.
			if reconcile && confirmed {
				return &ReconcileWithConfirmError{}
			}
			pr, err := parsePR(args[0])
			if err != nil {
				return err
			}
			owner, repo, err := repoOf(cmd)
			if err != nil {
				return err
			}
			layout, err := state.Default()
			if err != nil {
				return err
			}
			// Briefed rather than ReadMeta, for the reason
			// `cr draft` gives: the draft this reads back lives
			// under rounds/<n>/, and a pull request no round has
			// been opened on has no <n>. §11.2 codes that 4.
			round, err := briefedRound(layout, owner, repo, pr)
			if err != nil {
				return err
			}
			// §9.3.2: `cr post` is the command that anchors, and the
			// section exempts `--reconcile` from this refusal by
			// name — which is what says the rest of the command is
			// bound by it. The exemption is this return standing
			// above the refusal, and it is the whole of what makes
			// §9.3.2's sentence true of the built flag: what
			// §8.4.4 does anchors nothing, reads the pull request,
			// and writes only cr's own account of what it found.
			if reconcile {
				return reconcilePost(out, layout, &round)
			}
			if err := round.RefuseStale(); err != nil {
				return err
			}
			return buildReview(out, layout, owner, repo, pr, &round.Meta, confirmed)
		},
	}
	cmd.Flags().Bool("confirm", false, "perform the network write (§8.5.2)")
	cmd.Flags().Bool("reconcile", false, "resolve an unknown posting outcome (§8.4.4)")
	return cmd
}

// buildReview is §7.2.2: every record's grade recomputed and §6.3 re-applied
// before the payload is built, so no draft edit can turn an `argued` record
// into a posted assertion.
//
// The order is the whole of the criterion. The draft is read first, so the
// reviewer's verbs are in hand and §7.2's location row has moved the anchors it
// admitted. The grade is recomputed next, because §6.2's `probed` row reads the
// anchor: a grade computed before the move would keep a probe's support for a
// range that no longer holds its target. §7.2's `kind` row is then held to that
// recomputed grade, so a hardening that grade does not support aborts.
// §4.1.4's forcing and §6.3's are applied over what survived, in that order,
// and RefuseArguedAssertion is asked last — over the records that are about to
// become comments rather than over the rule that was just applied, which is
// invariant 4's shape.
//
// Nothing here writes. §7.3.1 has `cr post` without `--confirm` write no triage
// event, §8.5.2 has it perform no network write, and the states ingestDraft
// stamps on a discarded record stay in memory: this run reads the round,
// validates it, and reports.
//
// The gate is asked last, and that ordering is §8.5.1's and §11.2's together.
// Every refusal above — an `argued` assertion, a severity outside §5.4's bound
// for its gap probe, a comment count over §1.6.2's cap, a suggestion or a
// comment GitHub could not place — happens before the flag is looked
// at, so an invalid payload exits 1 whether or not `--confirm` was given, and
// only a payload that passed all of them reaches either branch of the gate.
func buildReview(
	out *writer, l state.Layout,
	owner, repo string, pr int, round *state.Meta, confirmed bool,
) error {
	records, err := roundFindingsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return err
	}
	// §8.3.1, before the draft is read: a round whose review was created or
	// adopted sends no second one, whatever its draft now says.
	if err := refusePostedRound(l, round, records); err != nil {
		return err
	}
	grading, err := readRoundGrading(l, owner, repo, pr, round)
	if err != nil {
		return err
	}
	// §9.1.1's journal of this run, whose actor is the one that sends: it
	// is published only by a confirmed send, beside the records it moved.
	journal := finding.NewJournal(finding.ActorPostConfirm, round.Head, time.Now())
	triage, err := ingestDraft(l, owner, repo, pr, round, records, journal)
	if err != nil {
		return err
	}
	// §7.2.2 over every record, after the draft's anchors were applied.
	if err := grading.regradeTriaged(round, records, &triage.Triage); err != nil {
		return err
	}
	queued := retypeForDraft(postedRecords(records), &triage.Triage)
	if len(queued) == 0 {
		// §7.2's discards took every queued record, so there is nothing
		// to post and the run settles them without a review.
		if len(triage.discarded()) > 0 {
			return settleDiscards(out, l, round, records, &triage, journal, confirmed)
		}
		return emptyReview(round, records)
	}
	grading.forceUnmapped(round.Round, queued)
	// §3.6.6 again, immediately before the payload is built: a note
	// retracted after the draft was rendered takes the assertion register
	// away from the records resting on it in this payload too.
	held := grading.holdWithdrawn(queued)
	// §6.3.1's third moment. The ids it moves are not kept: a dry run
	// writes nothing, and a confirmed send leaves no later moment to count.
	forced, _ := grading.forceQuestions(queued)
	if err := finding.RefuseArguedAssertion(queued); err != nil {
		return err
	}
	// §5.4.4 and §5.4.5 again, over the severities the draft's triage left
	// the queued comments carrying.
	if err := refuseGapSeverities(grading.roundEvidence, round, queued); err != nil {
		return err
	}
	// §1.6.2, over the same queued comments and before anything below can
	// reach GitHub: a round over the cap is refused whether or not
	// --confirm was given, and nothing is dropped to make it fit.
	if err := refuseOverCap(l, owner, repo, queued); err != nil {
		return err
	}
	// §8.4.1's pre-validation, over the comments the call would carry
	// rather than over the records the round recorded — which is why it
	// stands after the forcings and the draft's discards, and before
	// anything is rendered.
	if err := validatePositions(owner, repo, pr, round, queued, triage.Preserved); err != nil {
		return err
	}
	review, warnings, err := warnedPayload(l, owner, repo, pr, round, queued, triage.Preserved)
	if err != nil {
		return err
	}
	// §8.5.1: without `--confirm` the run prints the payload it built,
	// reports that it sent nothing, and exits 0.
	if !confirmed {
		return out.emit(&postResult{
			Round: round.Round, Comments: commentedRecords(review, queued),
			Payload: review, Discarded: discardedIDs(&triage), Forced: forced, Withdrawn: held,
			Warnings: warnings, Honesty: postDisclosures(round), posting: posting{Posted: false, ConfirmGiven: false},
		})
	}
	// §8.5.2 and §8.5.3: the permission travels as a value minted from the
	// flag and from nothing else, and the send path takes one — so there is
	// no shape of this call that omits it, and no second input a setting, a
	// variable, a profile field or an alias could arrive through.
	sender := &sending{
		layout: l, round: round, review: review, records: records,
		queued: queued, forced: forced, withdrawn: held, warnings: warnings, triage: &triage, journal: journal,
	}
	return sender.send(out, gh.Confirm(confirmed))
}

// postedRecords are the records §8.3 posts: the ones §9.1 still holds in
// `queued` once §7.2's discards have been read out of the draft.
//
// A discard is left out of the payload here, so a block the reviewer deleted
// is not posted. Its waiver and its stored `discarded` state are written only
// by a confirmed send, through the same waiveDiscards `cr draft` calls, because
// this command without `--confirm` writes nothing.
func postedRecords(records []*finding.Finding) []*finding.Finding {
	queued := make([]*finding.Finding, 0, len(records))
	for _, record := range records {
		if record.State == finding.StateQueued {
			queued = append(queued, record)
		}
	}
	return queued
}

// EmptyReviewError is the refusal of a review that would carry no comment: the
// round holds no record in `queued` once §7.2's discards are read out of the
// draft, and this run's draft discarded none of them — so none was ever drafted,
// or an earlier run already stored every discard. A round whose review went out
// is PostedRoundError's, and one whose discards this run reads is settled by
// settleDiscards.
//
// §8.3.1 sends a round's comments as one review, and a review holding no
// comment would notify the author of nothing. §8.4.4 is the other reason: a
// review holding no comment has a hash any other
// empty review shares, so a send of one whose outcome cr never learned could not
// be reconciled at all.
//
// It is refused whether or not `--confirm` was given, before the gate, so the
// dry run says what the confirmed run would. §11.2 codes it 4: the command line
// is right, and what refuses is where the round's records stand.
type EmptyReviewError struct {
	// Owner, Repo, and PR name the pull request whose round was refused.
	Owner string
	Repo  string
	PR    int
	// Round is the round that holds nothing to post.
	Round int
	// Posted and Discarded are how many of its records §9.1 holds in each
	// state, as this run read them with the draft's discards applied.
	Posted    int
	Discarded int
}

func (e *EmptyReviewError) Error() string {
	return fmt.Sprintf(
		"%s/%s#%d round %d holds no queued record, so its review would carry no comment "+
			"(%d posted, %d discarded): §8.3.1 sends a round's comments as one review and "+
			"cr sends no review without one",
		e.Owner, e.Repo, e.PR, e.Round, e.Posted, e.Discarded,
	)
}

// emptyReview is the EmptyReviewError for a round, counting its records.
func emptyReview(round *state.Meta, records []*finding.Finding) error {
	refused := &EmptyReviewError{Owner: round.Owner, Repo: round.Repo, PR: round.PR, Round: round.Round}
	for _, record := range records {
		switch record.State {
		case finding.StatePosted:
			refused.Posted++
		case finding.StateDiscarded:
			refused.Discarded++
		}
	}
	return refused
}

// PostedRoundError is §8.3.1's refusal of a second review for a round: the
// round already has the one review its comments are posted as, created by
// `cr post --confirm` or adopted by `cr post --reconcile`.
//
// It is refused before the draft is read and whatever the draft now says. A
// reviewer who clears a `wrong` after the round posted, or a record queued into
// the round since, would otherwise reach the author as a second review of the
// same round, which is the notification §8.3.1 exists to keep to one.
//
// A round still carrying `post_unresolved` is not refused here: whether its
// review exists is what cr has not learned, and UnresolvedPostError refuses its
// send with the step that learns it. It is refused whether or not `--confirm`
// was given, before the gate, so the dry run says what the confirmed run would.
// §11.2 codes it 4: the command line is right, and what refuses is where the
// round stands.
type PostedRoundError struct {
	// Owner, Repo, and PR name the pull request whose round was refused.
	Owner string
	Repo  string
	PR    int
	// Round is the round whose review was posted.
	Round int
	// Posted is how many of the round's records §9.1 holds in `posted`.
	Posted int
	// PayloadHash is §8.3.3's hash of the review the round was posted as,
	// as the round summary records it, and empty when it records none.
	PayloadHash string
}

func (e *PostedRoundError) Error() string {
	return fmt.Sprintf(
		"%s/%s#%d round %d is posted: its review was created or adopted (payload hash %q, "+
			"%d record(s) posted), and §8.3.1 posts a round's comments as one review, so cr "+
			"sends no second review for this round whatever its draft now says",
		e.Owner, e.Repo, e.PR, e.Round, e.PayloadHash, e.Posted,
	)
}

// refusePostedRound is PostedRoundError for a round that has its review.
//
// The review is read off what only its creation or adoption writes: the round
// summary's payload hash, which writeAdopted records for both, and records in
// `posted`, which §9.1 lets only those two moves store. Either is enough, so a
// summary lost or a record rewritten cannot reopen the round to a second send.
// An empty hash is not a review's: storeDiscards records it for a confirmed
// round whose draft discarded every record, and no review was sent for it.
func refusePostedRound(l state.Layout, round *state.Meta, records []*finding.Finding) error {
	if round.PostUnresolved {
		return nil
	}
	hash, _, err := state.ReadRoundSection[string](
		l, round.Owner, round.Repo, round.PR, round.Round, state.FileSummary, summaryPayloadHash)
	if err != nil {
		return err
	}
	posted := postedCount(records)
	if hash == "" && posted == 0 {
		return nil
	}
	return &PostedRoundError{
		Owner: round.Owner, Repo: round.Repo, PR: round.PR, Round: round.Round,
		Posted: posted, PayloadHash: hash,
	}
}

// postDisclosures is what a `cr post` run that sends nothing tells its reader
// before the payload: §8.4.4's `post_unresolved`, when the round carries it.
// It is empty and never nil.
func postDisclosures(round *state.Meta) []string {
	return append(make([]string, 0, 1), unresolvedDisclosure(round)...)
}

// discardedIDs are the ids of the records this run's draft discards, in the
// order the verbs are read, never nil.
func discardedIDs(triage *triaged) []string {
	discarded := triage.discarded()
	ids := make([]string, 0, len(discarded))
	for _, record := range discarded {
		ids = append(ids, record.ID)
	}
	return ids
}

// settleDiscards is `cr post` over a round whose draft discards every record it
// still held in `queued`: there is no comment to post, so no review is built and
// nothing reaches the network, and a confirmed run settles the discards.
//
// Settling is what a confirmed send writes for its discards once its call has
// returned, less the call: §7.4's waivers through waiveDiscards, §9.1's
// `queued` → `discarded` row under `cr post --confirm`, which lists it, and
// §7.3.1's one outcome event per queued record. The order is send's, for its
// reason — a waiver goes ahead of the discard it waives.
//
// The dry run writes nothing, per §8.5.1 and §7.3.1, and reports the discards
// the confirmed run would store. A round still carrying `post_unresolved` is
// refused its settling, as it is refused a send: the earlier call may have
// posted records this draft now discards, and §8.4.4's adoption is the step
// that learns it.
func settleDiscards(
	out *writer, l state.Layout, round *state.Meta, records []*finding.Finding,
	triage *triaged, journal *finding.Journal, confirmed bool,
) error {
	result := &postResult{
		Round: round.Round, Comments: make([]postedComment, 0), Discarded: discardedIDs(triage),
		Forced: make(finding.Forcings, 0), Withdrawn: make(finding.Withdrawn, 0),
		Honesty: postDisclosures(round),
		posting: posting{Posted: false, ConfirmGiven: confirmed},
	}
	if !confirmed {
		return out.emit(result)
	}
	owner, repo, pr := round.Owner, round.Repo, round.PR
	if round.PostUnresolved {
		return &UnresolvedPostError{Owner: owner, Repo: repo, PR: pr, Round: round.Round}
	}
	if err := waiveDiscards(l, owner, repo, pr, triage.discarded()); err != nil {
		return err
	}
	if err := storeDiscards(l, round, records, journal); err != nil {
		return err
	}
	if err := recordPostTriage(l, owner, repo, pr, round, triage.settled()); err != nil {
		return err
	}
	return out.emit(result)
}

// storeDiscards publishes the round's records with the draft's discards in
// `discarded`, §9.1.1's lines for the moves, and the round summary §10.3 has
// `cr post` finalise, under one §2.3.1 lock.
//
// The summary is finalised as a confirmed round that sent nothing: §8.5.4's
// confirmation, no posted record, no comment, the discard counts, and an empty
// payload hash, since no payload was built. The empty hash is what
// refusePostedRound reads as a round without a review.
func storeDiscards(
	l state.Layout, round *state.Meta, records []*finding.Finding, journal *finding.Journal,
) error {
	comments, err := commentCount(l, round.Owner, round.Repo, 0)
	if err != nil {
		return err
	}
	held, err := l.LockPR(round.Owner, round.Repo, round.PR)
	if err != nil {
		return err
	}
	stamp := state.Stamp{Head: round.Head, Round: round.Round}
	writes := []func() error{
		func() error { return journal.Write(held) },
		func() error { return state.ReplaceStamped(held, state.FileFindings, stamp, records) },
		func() error { return writeSummary(held, round.Round, ownerPost, postCounts(postedCount(records), "")) },
		func() error { return writeSummary(held, round.Round, ownerComments, comments) },
		func() error { return writeSummary(held, round.Round, ownerDiscards, discardCounts(records)) },
	}
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

// refuseOverCap is §1.6.2's block at posting: the queued comments measured
// against post.max_comments through finding.CommentCapFor, the one call
// §7.1.4's draft header reads its count from, so the number the reviewer
// triaged against is the number that refuses here.
//
// The cap is resolved the way the draft resolves it, from the same layers, for
// the same reason buildPayload gives. The refusal is CommentCapExceededError,
// which the exit table codes 1 with the triage-to-fit hint.
func refuseOverCap(l state.Layout, owner, repo string, queued []*finding.Finding) error {
	settings, err := resolveDraftSettings(l, owner, repo)
	if err != nil {
		return err
	}
	return finding.CommentCapFor(queued, settings.maxComments).Err()
}

// warnedPayload is buildPayload and §8.2.3's warnings over the suggestions
// that payload carries, both read from the same preserved bodies: a fence the
// reviewer re-indented in draft.md, after the last `cr draft` included, is
// warned about here and still sent as written.
func warnedPayload(
	l state.Layout, owner, repo string, pr int, round *state.Meta,
	queued []*finding.Finding, preserved map[string]string,
) (*post.Review, []string, error) {
	review, err := buildPayload(l, owner, repo, pr, round, queued, preserved)
	if err != nil {
		return nil, nil, err
	}
	warnings, err := indentationWarnings(owner, repo, pr, round, queued, preserved)
	if err != nil {
		return nil, nil, err
	}
	return review, warnings, nil
}

// buildPayload renders every queued record's §8.1.3 comment and assembles
// §8.3.1's single review from them.
//
// The bodies come from draft.PostBodies, which is commentOf — the one path
// `cr draft` renders a block through — so the cr-owned regions the author reads
// are byte for byte the ones the reviewer approved in the draft. The settings
// and the provenance sources are read the way the draft reads them, and for the
// same reason: a region regenerated under a different language or a different
// probe cap would be a second answer to what the draft already said.
func buildPayload(
	l state.Layout, owner, repo string, pr int, round *state.Meta,
	queued []*finding.Finding, preserved map[string]string,
) (*post.Review, error) {
	settings, err := resolveDraftSettings(l, owner, repo)
	if err != nil {
		return nil, err
	}
	sources, err := draftProvenances(l, owner, repo, pr, round, queued)
	if err != nil {
		return nil, err
	}
	sources.MaxProbeInput = settings.maxProbeInput
	bodies, err := draft.PostBodies(queued, settings.lang, sources, preserved)
	if err != nil {
		return nil, err
	}
	review := post.Build(queued, bodies)
	review.CommitID = round.Head
	if err := discloseInBody(l, owner, repo, pr, round, review); err != nil {
		return nil, err
	}
	return review, nil
}

// discloseInBody fills §8.4.3's review body: §4.5.4's disclosure, and beneath
// it the payload hash as an HTML comment.
//
// The hash is taken before the body is set and over the comments alone, which
// is §8.3.3's pre-image and what makes this orderable at all: a hash over the
// body it is embedded in would have to contain its own digest.
//
// The lenses come from lensesOf and from nowhere else, and that is the point of
// the call rather than an implementation detail. §4.5.4's disclosure now has two
// readers — `cr status`, which prints it to the reviewer, and this, which sends
// it to the author — and two derivations of "what did not look" that can
// disagree is precisely the failure the section exists to prevent. The cost is
// stated plainly: lensesOf computes §4.3.1's and §4.4.1's halves the way
// `cr review` computes them, over a symbol index on the round's head and the
// round's own diff, so `cr post` now needs the repository and one `gh` read on
// every run. That is the price of the criterion, which names the reinvention
// half as something the author is owed; the alternative is a posted body quietly
// shorter than the terminal's, which is a dishonesty of exactly the kind §4.5.4
// is about.
func discloseInBody(
	l state.Layout, owner, repo string, pr int, round *state.Meta, review *post.Review,
) error {
	hash, err := review.Hash()
	if err != nil {
		return err
	}
	axes, lenses, err := lensesOf(l, owner, repo, pr, round)
	if err != nil {
		return err
	}
	review.Body = render.ReviewBody(axes.Active, lenses.Disclosures(), hash)
	return nil
}

// commentedRecords are the payload's comments as `cr post` reports them, in
// payload order, never nil.
//
// The register is read off the record rather than off the comment, because a
// comment carries none: §8.1.4's label is inside the body by then, and a
// reporter that recovered the kind by looking for it would be parsing cr's own
// rendering to answer a question the record already answers.
func commentedRecords(review *post.Review, queued []*finding.Finding) []postedComment {
	registers := make(map[string]finding.Kind, len(queued))
	for _, record := range queued {
		registers[record.ID] = record.Kind
	}
	comments := make([]postedComment, 0, len(review.Comments))
	for i := range review.Comments {
		id := review.Comments[i].Record
		comments = append(comments, postedComment{ID: id, Kind: registers[id]})
	}
	return comments
}
