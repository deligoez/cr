package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// notSettledError reports a resolve asked for a record nobody has settled.
//
// §9.6.1 refuses it because resolving is how a thread stops being visible, and
// a thread resolved over an open concern is the concern hidden rather than
// answered — the one outcome worse than leaving it open, because nobody will
// look again.
type notSettledError struct {
	// ID is the record, and State where it actually stands.
	ID, State string
}

func (e *notSettledError) Error() string {
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

// newWithdrawCmd retracts a posted concern and resolves its thread (§9.6.2).
//
// It posts no prose, and §9.6.3 says why that is a constraint rather than a
// gap: §8.1.2 gives cr one channel for a body a human wrote, and a flag here
// carrying text to GitHub would be a second. The reviewer who wants to tell
// the author why writes that reply themselves, exactly as §7.2.3 already has
// them write every comment cr does not compose.
func newWithdrawCmd(out *writer) *cobra.Command {
	var confirm bool
	cmd := &cobra.Command{
		Use:   "withdraw " + prPlaceholder + " <record-id>",
		Short: "Retract a posted concern and resolve its thread",
		Args:  prArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return settleRecord(out, cmd, args, settling{
				action: "withdraw", confirm: confirm,
			})
		},
	}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "send the retraction to GitHub")
	return cmd
}

// settling is what the two commands differ by.
type settling struct {
	action  string
	confirm bool
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
	records, err := state.ReadStamped[*finding.Finding](
		layout, owner, repo, pr, state.FileFindings, round.Round)
	if err != nil {
		return err
	}
	held := recordNamed(records, args[1])
	if held == nil {
		return &unknownRecordError{ID: args[1], Round: round.Round}
	}
	// §6.1 stamps `thread_id` when a record is posted, so the thread is
	// read off the record rather than looked up: a record with none never
	// reached GitHub, and §9.6 has nothing there to act on.
	if held.ThreadID == "" {
		return &noThreadError{ID: held.ID}
	}
	if how.action == "resolve" &&
		held.State != finding.StateAnswered && held.State != finding.StateAddressed {
		return &notSettledError{ID: held.ID, State: held.State.String()}
	}
	if how.action == "withdraw" && held.State != finding.StatePosted {
		return &notSettledError{ID: held.ID, State: held.State.String()}
	}

	result := &settleResult{
		ID: held.ID, Thread: held.ThreadID, Action: how.action,
		State:   held.State.String(),
		posting: posting{ConfirmGiven: how.confirm},
	}
	if !how.confirm {
		return out.emit(result)
	}
	return sendSettlement(out, layout, owner, repo, pr, &round, records, held, how, result)
}

// sendSettlement performs §9.6's writes: the reply when there is one, the
// resolution, and then the record's own move.
//
// The order is what §8.4's unknown outcome makes it. GitHub is written first
// and cr's state second, so a run that dies between them leaves a thread
// resolved and a record still posted — which `cr recheck` reads back and a
// human can settle. The reverse order would leave a record marked withdrawn
// over a thread still open on the pull request, and nothing would ever look at
// it again.
func sendSettlement(
	out *writer, l state.Layout, owner, repo string, pr int, round *state.Round,
	records []*finding.Finding, held *finding.Finding,
	how settling, result *settleResult,
) error {
	if err := gh.Confirm(true).ResolveThread(held.ThreadID); err != nil {
		return err
	}
	result.Posted = true
	if how.action != "withdraw" {
		result.State = held.State.String()
		return out.emit(result)
	}

	journal := finding.NewJournal(finding.ActorWithdrawConfirm, round.Head, time.Now())
	if err := journal.Move(held.ID, finding.Existing(held.State), finding.StateWithdrawn); err != nil {
		return err
	}
	held.State = finding.StateWithdrawn
	if err := writeWithdrawal(l, owner, repo, pr, round, records, journal); err != nil {
		return err
	}
	result.State = held.State.String()
	return out.emit(result)
}

// writeWithdrawal stores §9.6.2's move under §2.3.1's lock, with the journal
// line §9.1.1 requires beside it.
func writeWithdrawal(
	l state.Layout, owner, repo string, pr int, round *state.Round,
	records []*finding.Finding, journal *finding.Journal,
) error {
	lock, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return err
	}
	stamp := state.Stamp{Head: round.Head, Round: round.Round}
	writes := []func() error{
		func() error { return journal.Write(lock) },
		func() error { return state.ReplaceStamped(lock, state.FileFindings, stamp, records) },
	}
	for _, write := range writes {
		if err := write(); err != nil {
			_ = lock.Unlock()
			return err
		}
	}
	return lock.Unlock()
}
