package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/post"
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
	// Forced is §6.3.2's count per class over the records the payload
	// holds. It is reported here as well as by `cr draft` because this is
	// §6.3.1's last application — the only one that sees the payload the
	// author will actually receive.
	Forced finding.Forcings `json:"forced_to_question"`
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
	return text + r.Forced.Disclosure()
}

// unbuiltPostFlags are the two §11 flags this command registers and does not
// yet act on, in the order §11 writes them.
//
// They refuse rather than being ignored, which is the same choice `cr config
// --resolved` made and matters more here: §8.5.2 makes `--confirm` the whole of
// the network-write gate, and a build that accepted the flag and quietly
// validated would teach a caller that `--confirm` is satisfied by a run that
// sent nothing. The refusal names the flag, so the caller is told what is
// missing rather than that the command they just used is.
var unbuiltPostFlags = []string{"confirm", "reconcile"}

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
// Nothing here mints §8.5's confirmation. The gate's own file is the deliberate
// widening of nowrite_test.go's mintSites, and it arrives with the behaviour
// rather than with the surface.
func newPostCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "post " + prPlaceholder,
		Short: "Validate and post the review",
		Args:  prArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, unbuilt := range unbuiltPostFlags {
				if asked, err := cmd.Flags().GetBool(unbuilt); err == nil && asked {
					return notImplementedFor(cmd, unbuilt)
				}
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
			// bound by it. The exemption is reached above, where
			// every unbuilt flag refuses before any state is read,
			// and the flag's behaviour has to keep arriving on that
			// side of this line.
			if err := round.RefuseStale(); err != nil {
				return err
			}
			return buildReview(out, layout, owner, repo, pr, &round.Meta)
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
func buildReview(out *writer, l state.Layout, owner, repo string, pr int, round *state.Meta) error {
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
	review, err := buildPayload(l, owner, repo, pr, round, queued, triage.Preserved)
	if err != nil {
		return err
	}
	return out.emit(&postResult{
		Round: round.Round, Comments: commentedRecords(review, queued), Forced: forced,
	})
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
	return post.Build(queued, bodies), nil
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
