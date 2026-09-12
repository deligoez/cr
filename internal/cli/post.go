package cli

import (
	"strconv"

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
// It carries no §12.6 `posted` field, and that is the contract rather than an
// omission. §12.6 gives the field to the commands that could have performed the
// network write, which §8.5.2 makes exactly the commands that mint a
// confirmation; this one mints none, so a `"posted": false` here would report a
// decision nothing made. The field arrives with the gate, in the commit that
// builds it.
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
	// Forced is §6.3.2's count per class over the records the payload
	// holds. It is reported here as well as by `cr draft` because this is
	// §6.3.1's last application — the only one that sees the payload the
	// author will actually receive.
	Forced finding.Forcings `json:"forced_to_question"`
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
func (r *postResult) Text(w *writer) string {
	text := "built a review of " + w.accent(strconv.Itoa(len(r.Comments))) +
		" comment(s) for round " + strconv.Itoa(r.Round) + "\n"
	for _, comment := range r.Comments {
		text += comment.ID + ": " + string(comment.Kind) + "\n"
	}
	return text + r.payload() + r.Forced.Disclosure() + "\n" + r.line(w)
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
	text := "\n" + r.Payload.Body + "\n"
	for i := range r.Payload.Comments {
		comment := &r.Payload.Comments[i]
		text += "\n" + comment.Record + " " + comment.Path + ":" +
			strconv.Itoa(comment.Line) + " " + string(comment.Side) + "\n" + comment.Body + "\n"
	}
	return text + "\n"
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
			if reconcile, err := cmd.Flags().GetBool("reconcile"); err == nil && reconcile {
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
// The order is the whole of the criterion. The grade is recomputed first,
// because §7.2's `kind` row admits a hardening on "the recomputed grade" and
// reading the marker against the grade the round was recorded with would admit
// an assertion the evidence no longer supports. The draft is read next, so the
// reviewer's verbs and edits are in hand. §4.1.4's forcing and §6.3's are then
// applied over what survived, in that order, and RefuseArguedAssertion is asked
// last — over the records that are about to become comments rather than over
// the rule that was just applied, which is invariant 4's shape.
//
// Nothing here writes. §7.3.1 has `cr post` without `--confirm` write no triage
// event, §8.5.2 has it perform no network write, and the states ingestDraft
// stamps on a discarded record stay in memory: this run reads the round,
// validates it, and reports.
//
// The gate is asked last, and that ordering is §8.5.1's and §11.2's together.
// Every refusal above — an `argued` assertion, a suggestion GitHub could not
// place, a comment count over §1.6.2's cap — happens before the flag is looked
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
	grading, err := readRoundGrading(l, owner, repo, pr, round.Round)
	if err != nil {
		return err
	}
	grading.regrade(round, records)
	triage, err := ingestDraft(l, owner, repo, pr, round, records)
	if err != nil {
		return err
	}
	queued := retypeForDraft(postedRecords(records), &triage.Triage)
	grading.forceUnmapped(round.Round, queued)
	forced := finding.ForceQuestions(queued)
	if err := finding.RefuseArguedAssertion(queued); err != nil {
		return err
	}
	// §8.4.1's pre-validation, over the comments the call would carry
	// rather than over the records the round recorded — which is why it
	// stands after the forcings and the draft's discards, and before
	// anything is rendered.
	if err := validatePositions(owner, repo, pr, round, queued); err != nil {
		return err
	}
	review, err := buildPayload(l, owner, repo, pr, round, queued, triage.Preserved)
	if err != nil {
		return err
	}
	// §8.5.1: without `--confirm` the run prints the payload it built,
	// reports that it sent nothing, and exits 0.
	if !confirmed {
		return out.emit(&postResult{
			Round: round.Round, Comments: commentedRecords(review, queued),
			Payload: review, Forced: forced, posting: posting{Posted: false},
		})
	}
	// §8.5.2 and §8.5.3: the permission travels as a value minted from the
	// flag and from nothing else, and the send path takes one — so there is
	// no shape of this call that omits it, and no second input a setting, a
	// variable, a profile field or an alias could arrive through.
	sender := &sending{
		layout: l, round: round, review: review, records: records,
		queued: queued, forced: forced, triage: &triage,
	}
	return sender.send(out, gh.Confirm(confirmed))
}

// postedRecords are the records §8.3 posts: the ones §9.1 still holds in
// `queued` once §7.2's discards have been read out of the draft.
//
// A discard is read here and never written, which is the difference between
// this command and `cr draft`. §7.2's two discard verbs are observable in the
// file either way, and `cr draft` is the command that acts on them — it walks
// the record to `discarded` and writes its waiver. What `cr post` owes the
// reviewer is that a block they deleted is not posted, and leaving it out of
// the payload is the whole of that.
func postedRecords(records []*finding.Finding) []*finding.Finding {
	queued := make([]*finding.Finding, 0, len(records))
	for _, record := range records {
		if record.State == finding.StateQueued {
			queued = append(queued, record)
		}
	}
	return queued
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
	if err := discloseInBody(l, owner, repo, pr, round, settings.lang, review); err != nil {
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
	l state.Layout, owner, repo string, pr int, round *state.Meta,
	lang render.Lang, review *post.Review,
) error {
	hash, err := review.Hash()
	if err != nil {
		return err
	}
	axes, lenses, err := lensesOf(l, owner, repo, pr, round)
	if err != nil {
		return err
	}
	body, err := render.ReviewBody(lang, axes.Active, lenses.Disclosures(), hash)
	if err != nil {
		return err
	}
	review.Body = body
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
