package cli

import (
	"errors"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// verifyEvidenceFlag is §9.5.5's required flag.
const verifyEvidenceFlag = "evidence"

// errEmptyEvidence is §9.5.5's refusal of a judgement with nothing behind it.
//
// The flag is required, so this catches the other half: `--evidence ""`
// satisfies cobra and says nothing. A verdict stored with empty evidence would
// read back as a judgement somebody made and nobody can check, which is the
// shape §6.2's whole grade ladder exists to keep out of the record.
// It is left unmapped, so §11.2 codes it 2.
var errEmptyEvidence = errors.New(
	"--evidence is empty, and §9.5.5 requires a judgement to say what it rests on: give it the " +
		"reply, the diff, or the probe result the verdict was read from")

// unknownRecordError reports a record id no round of the pull request holds.
type unknownRecordError struct {
	// ID is what was named.
	ID string
}

func (e *unknownRecordError) Error() string {
	return "no record " + e.ID + " is stored for this pull request, in any round"
}

// Hint is §12.4's next actionable step.
func (e *unknownRecordError) Hint() string {
	return "run cr recheck to list the posted records, or cr status for the current round's"
}

// verifyResult is what `cr verify` reports: the record, the verdict carried in,
// and where the record now stands.
type verifyResult struct {
	// ID is the record judged.
	ID string `json:"id"`
	// Verdict is §9.5.5's verb, as given.
	Verdict string `json:"verdict"`
	// State is the record's state after the judgement, which is the
	// verdict's own name for two of the three and `posted` for `standing`.
	State string `json:"state"`
	// Moved reports whether the record changed state, so a reader can tell
	// a judgement that settled one from a judgement that recorded it still
	// open.
	Moved bool `json:"moved"`
	// Honesty carries §9.3.1's head comparison.
	Honesty []string `json:"honesty"`
}

func (r *verifyResult) Text(w *writer) string {
	text := w.accent(r.ID) + " " + r.Verdict
	if !r.Moved {
		text += ", still posted"
	} else {
		text += ", now " + r.State
	}
	return text + w.disclose("\n", "", r.Honesty...)
}

// newVerifyCmd records the agent's judgement about one posted record (§9.5.5).
//
// The verb is an argument and never an inference. §9.5.6 is the whole reason
// this command exists as a separate step rather than as something `cr recheck`
// concludes: every signal cr can read is as consistent with a concern that was
// addressed as with one whose code was deleted, so the word has to be carried
// in from outside. Nothing here reads the evidence; it is stored for the human
// who reads the round back, the way §6.2.1 stores a record's own evidence.
func newVerifyCmd(out *writer) *cobra.Command {
	var evidence string

	cmd := &cobra.Command{
		Use:   "verify " + prPlaceholder + " <record-id> answered|addressed|standing",
		Short: "Record the agent's judgement about one posted record",
		Args:  prArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			pr, err := parsePR(args[0])
			if err != nil {
				return err
			}
			verdict, err := finding.ParseVerdict(args[2])
			if err != nil {
				return err
			}
			if evidence == "" {
				return errEmptyEvidence
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
			// §9.3.2: this command writes per-PR state.
			if err := round.RefuseStale(); err != nil {
				return err
			}
			return verifyRecord(out, layout, owner, repo, pr, &round, args[1], verdict, evidence)
		},
	}
	cmd.Flags().StringVar(&evidence, verifyEvidenceFlag, "",
		"what the judgement rests on, stored and never parsed")
	// The error is a flag that does not exist, which the line above rules
	// out.
	_ = cmd.MarkFlagRequired(verifyEvidenceFlag)

	return cmd
}

// verifyRecord applies one verdict, under §2.3.1's lock.
//
// The verdict is appended whatever it decides, including `standing`. §9.5.5
// wants that line: a concern somebody read and left open is a different fact
// from one nobody read, and the two were indistinguishable before v0.5 because
// both sat in `posted`.
func verifyRecord(
	out *writer, l state.Layout, owner, repo string, pr int, round *state.Round,
	id string, verdict finding.Verdict, evidence string,
) error {
	held, err := sentRecord(l, owner, repo, pr, id)
	if err != nil {
		return err
	}
	if verdict == finding.VerdictAnswered && held.Kind != finding.KindQuestion {
		return &finding.AnsweredNeedsAQuestionError{Record: id, Kind: string(held.Kind)}
	}

	journal := finding.NewJournal(finding.ActorVerify, round.Head, time.Now())
	from, moved := held.State, false
	if to, changes := verdict.State(); changes {
		if err := journal.Move(id, finding.Existing(held.State), to); err != nil {
			return err
		}
		held.State, moved = to, true
	}

	line := finding.VerdictRecord{
		Record: id, Verdict: verdict.String(), Evidence: evidence,
		Head: round.Head, Round: round.Round, At: time.Now(),
	}
	if err := writeVerdict(l, owner, repo, pr, held, from, moved, journal, &line); err != nil {
		return err
	}
	return out.emit(&verifyResult{
		ID: id, Verdict: verdict.String(), State: held.State.String(), Moved: moved,
		Honesty: []string{round.Disclosure()},
	})
}

// writeVerdict performs §9.5.5's writes as one, under the per-PR lock.
//
// A verdict that moves the record changes its state in the line that holds it,
// through finding.MoveSent, which writes the journal first; `standing` moves
// nothing and writes no record. The verdict line is an append, because it is a
// history and replaces nothing.
func writeVerdict(
	l state.Layout, owner, repo string, pr int,
	record *finding.Finding, from finding.State, moved bool,
	journal *finding.Journal, line *finding.VerdictRecord,
) error {
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return err
	}
	writes := []func() error{
		func() error { return journal.Write(held) },
	}
	if moved {
		writes[0] = func() error {
			return finding.MoveSent(held, record.ID, from, record.State, journal)
		}
	}
	writes = append(writes, func() error {
		return state.AppendRecords(held, state.FileVerdicts, []finding.VerdictRecord{*line})
	})
	for _, write := range writes {
		if err := write(); err != nil {
			_ = held.Unlock()
			return err
		}
	}
	return held.Unlock()
}
