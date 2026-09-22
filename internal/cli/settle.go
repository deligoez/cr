package cli

import (
	"errors"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// notSettledError reports a record in a state §9.6's command does not act on:
// a resolve asked for a record nobody has settled, or a withdrawal for one
// that is not posted.
//
// §9.6.1 refuses the first because resolving is how a thread stops being
// visible, and a thread resolved over an open concern is the concern hidden
// rather than answered — the one outcome worse than leaving it open, because
// nobody will look again.
type notSettledError struct {
	// ID is the record, State where it actually stands, and Action the
	// command that refused it.
	ID, State, Action string
}

func (e *notSettledError) Error() string {
	if e.Action == "withdraw" {
		return "record " + e.ID + " is " + e.State +
			", and §9.6.2 withdraws only a record in posted"
	}
	return "record " + e.ID + " is " + e.State +
		", and §9.6.1 resolves a thread only for a record in answered or addressed"
}

// noThreadError reports a record cr holds no posted thread for.
type noThreadError struct{ ID string }

func (e *noThreadError) Error() string {
	return "record " + e.ID + " carries no posted thread, so §9.6 has nothing on GitHub to act on"
}

// settleResult is what `cr resolve` and `cr withdraw` report.
type settleResult struct {
	// ID is the record acted on, and Thread its GitHub node id.
	ID     string `json:"id"`
	Thread string `json:"thread"`
	// Action is `resolve` or `withdraw`.
	Action string `json:"action"`
	// Disposition is the verb a withdrawal was given, and Scope the file
	// of §7.4.4 its waiver is written to. Both are empty for a resolution.
	Disposition string `json:"disposition,omitempty"`
	Scope       string `json:"scope,omitempty"`
	// posting is §12.6's pair, embedded rather than restated: §9.6's two
	// commands can perform a network write, so their payload reports
	// whether one happened and whether `--confirm` was given, in the field
	// names one place declares.
	posting
	// State is where the record stands afterwards.
	State string `json:"state"`
}

func (r *settleResult) Text(w *writer) string {
	text := w.accent(r.ID) + " " + r.Action + " on thread " + r.Thread
	if r.Disposition != "" {
		text += " as " + r.Disposition + ", waived for the " + r.Scope
	}
	if !r.Posted {
		return text + "\nnot sent: §8.5 requires --confirm"
	}
	return text + ", now " + r.State
}

// newResolveCmd resolves a settled record's thread (§9.6.1).
func newResolveCmd(out *writer) *cobra.Command {
	var confirm bool
	cmd := &cobra.Command{
		Use:   "resolve " + prPlaceholder + " <record-id>",
		Short: "Resolve a settled record's thread",
		Args:  prArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return settleRecord(out, cmd, args, settling{
				action: "resolve", confirm: confirm,
			})
		},
	}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "send the resolution to GitHub")
	return cmd
}

// withdrawalVerbs are §9.6.2's two dispositions, spelled as `cr triage`
// spells them, so one decision has one word whichever side of posting it is
// made on.
var withdrawalVerbs = map[string]finding.Disposition{
	"wrong":    finding.DispositionWrong,
	"not-here": finding.DispositionNotHere,
}

// errWithdrawalVerb is §9.6.2's refusal of a withdrawal that does not say
// which it is. It is left unmapped, so §11.2 codes it 2.
var errWithdrawalVerb = errors.New(
	"a withdrawal names its disposition, wrong or not-here: §9.6.2 cannot tell a false concern " +
		"from a true one not worth the comment, and the two are waived and counted differently")

// newWithdrawCmd retracts a posted concern and resolves its thread (§9.6.2).
//
// It posts no prose, and §9.6.3 says why that is a constraint rather than a
// gap: §8.1.2 gives cr one channel for a body a human wrote, and a flag here
// carrying text to GitHub would be a second. The reviewer who wants to tell
// the author why writes that reply themselves, exactly as §7.2.3 already has
// them write every comment cr does not compose.
//
// The disposition is positional and required. A retraction is the strongest
// evidence about a class cr can collect, because the author read the concern,
// and whether it says "this was false" or "this was not worth saying" is the
// reviewer's knowledge and never something cr could infer.
func newWithdrawCmd(out *writer) *cobra.Command {
	var confirm bool
	cmd := &cobra.Command{
		Use:   "withdraw " + prPlaceholder + " <record-id> wrong|not-here",
		Short: "Retract a posted concern, waive it, and resolve its thread",
		Args:  prArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			disposition, named := withdrawalVerbs[args[2]]
			if !named {
				return errWithdrawalVerb
			}
			return settleRecord(out, cmd, args, settling{
				action: "withdraw", confirm: confirm, disposition: disposition,
			})
		},
	}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "send the retraction to GitHub")
	return cmd
}

// settling is what the two commands differ by.
type settling struct {
	action      string
	confirm     bool
	disposition finding.Disposition
}

// settleRecord is §9.6's shared half: find the record and its thread, hold it
// to the section's refusals, and either print what would be sent or send it.
func settleRecord(out *writer, cmd *cobra.Command, args []string, how settling) error {
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
	round, err := briefedRound(layout, owner, repo, pr)
	if err != nil {
		return err
	}
	// §9.3.2: both commands write per-PR state once confirmed.
	if err := round.RefuseStale(); err != nil {
		return err
	}
	held, err := sentRecord(layout, owner, repo, pr, args[1])
	if err != nil {
		return err
	}
	// §6.1 stamps `thread_id` when a record is posted, so the thread is
	// read off the record rather than looked up: a record with none never
	// reached GitHub, and §9.6 has nothing there to act on.
	if held.ThreadID == "" {
		return &noThreadError{ID: held.ID}
	}
	if how.action == "resolve" &&
		held.State != finding.StateAnswered && held.State != finding.StateAddressed {
		return &notSettledError{ID: held.ID, State: held.State.String(), Action: how.action}
	}
	if how.action == "withdraw" && held.State != finding.StatePosted {
		return &notSettledError{ID: held.ID, State: held.State.String(), Action: how.action}
	}

	result := &settleResult{
		ID: held.ID, Thread: held.ThreadID, Action: how.action,
		State:   held.State.String(),
		posting: posting{ConfirmGiven: how.confirm},
	}
	var retraction *withdrawal
	if how.action == "withdraw" {
		// Formed before anything is sent, dry run included: a key that
		// cannot be formed refuses here, with nothing on GitHub changed,
		// rather than after the thread is already resolved.
		retraction, err = withdrawalOf(owner, repo, pr, held, how.disposition)
		if err != nil {
			return err
		}
		result.Disposition, result.Scope = string(how.disposition), retraction.scope.String()
	}
	if !how.confirm {
		return out.emit(result)
	}
	return sendSettlement(out, layout, owner, repo, pr, &round, held, retraction, result)
}

// withdrawal is what §9.6.2 records beside the move: the waiver and the
// outcome the disposition names.
type withdrawal struct {
	waiver  finding.Waiver
	scope   finding.WaiverScope
	outcome finding.Outcome
}

// withdrawalOf forms a retraction's waiver and outcome, and writes nothing.
//
// The key is the one the record stamped when it was recorded (§9.2.4), so a
// withdrawal after a force-push needs no commit the clone may have lost. A
// record an earlier release stamped carries no stamped key, and only then is
// the head it was produced against read, lazily, through keyTrees.
func withdrawalOf(
	owner, repo string, pr int, held *finding.Finding, d finding.Disposition,
) (*withdrawal, error) {
	outcome, err := finding.WithdrawalOutcome(d)
	if err != nil {
		return nil, err
	}
	waiver, err := finding.WaiverForWithdrawal(keyTrees(owner, repo, pr, held.Head), held, d)
	if err != nil {
		return nil, err
	}
	scope, err := waiver.Scope()
	if err != nil {
		return nil, err
	}
	return &withdrawal{waiver: waiver, scope: scope, outcome: outcome}, nil
}

// sendSettlement performs §9.6's writes: the resolution, and for a
// withdrawal the waiver, the outcome and then the record's own move.
//
// The order is what §8.4's unknown outcome makes it. GitHub is written first
// and cr's state second, so a run that dies between them leaves a thread
// resolved and a record still posted — which `cr recheck` reads back and a
// human can settle. The reverse order would leave a record marked withdrawn
// over a thread still open on the pull request, and nothing would ever look at
// it again. Among cr's own writes the move is last: the waiver and the outcome
// event are both idempotent, so a run that dies before the move leaves a
// record still posted that the same command can finish, and none that dies
// after it can leave a withdrawn record with no waiver.
func sendSettlement(
	out *writer, l state.Layout, owner, repo string, pr int, round *state.Round,
	held *finding.Finding, retraction *withdrawal, result *settleResult,
) error {
	if err := gh.Confirm(true).ResolveThread(held.ThreadID); err != nil {
		return err
	}
	result.Posted = true
	if retraction == nil {
		result.State = held.State.String()
		return out.emit(result)
	}

	// §7.4.8's provenance and §7.3.1's key are the posting's: a posted
	// record stays in the round that posted it, so its own round and head
	// are that round's, and the outcome event replaces the `kept` the
	// posting wrote rather than joining it.
	if _, err := finding.Waive(l, owner, repo, &retraction.waiver, finding.WaiverProvenance{
		Round: held.Round, PR: pr, Head: held.Head, Reason: finding.WithdrawalReason,
	}); err != nil {
		return err
	}
	if err := finding.RecordOutcomes(l, owner, repo,
		[]finding.Settled{{Record: held, Outcome: retraction.outcome}},
		&finding.TriageOccasion{PR: pr, Round: held.Round, Head: held.Head, At: time.Now()},
	); err != nil {
		return err
	}
	journal := finding.NewJournal(finding.ActorWithdrawConfirm, round.Head, time.Now())
	if err := journal.Move(held.ID, finding.Existing(held.State), finding.StateWithdrawn); err != nil {
		return err
	}
	if err := writeWithdrawal(l, owner, repo, pr, held, journal); err != nil {
		return err
	}
	held.State = finding.StateWithdrawn
	result.State = held.State.String()
	return out.emit(result)
}

// writeWithdrawal stores §9.6.2's move under §2.3.1's lock, with the journal
// line §9.1.1 requires beside it, in the line that holds the record.
func writeWithdrawal(
	l state.Layout, owner, repo string, pr int, held *finding.Finding, journal *finding.Journal,
) error {
	lock, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return err
	}
	if err := finding.MoveSent(lock, held.ID, held.State, finding.StateWithdrawn, journal); err != nil {
		_ = lock.Unlock()
		return err
	}
	return lock.Unlock()
}
