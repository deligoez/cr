package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// draftResult is what `cr draft` has to report: where the draft was written,
// which round it belongs to, and how many records it holds.
//
// The blocks themselves are not handed back. §7.1 writes them to a file the
// reviewer is about to open and edit, so printing them into the caller's
// terminal would repeat a document that already exists at a path — and the path
// is the one thing the caller cannot derive, since §2.2 puts it under the state
// root rather than beside the repository.
type draftResult struct {
	// Path is the rounds/<n>/draft.md the run wrote.
	Path string `json:"path"`
	// Round is the round it belongs to. §9.3.5 makes every round's
	// artefacts its own, so which one was written is part of the answer.
	Round int `json:"round"`
	// Queued is how many records §7.1 rendered into it, which is also how
	// many §9.1 now holds in `queued`.
	Queued int `json:"queued"`
	// Forced is §6.3.2's count per class over the records this draft
	// holds, which is also what reached summary.json. It is a field on the
	// payload rather than a sentence alone, so an agent reading the
	// document gets the numbers and not only the prose.
	Forced finding.Forcings `json:"forced_to_question"`
}

// Text names the count, the round, and the file to open, and then §6.3.2's
// forcing count.
//
// The forcing line is printed on every run, whatever the flags say. §11.1
// exempts it from `--quiet` by name, and a disclosure that is only printed
// sometimes is a disclosure the reader cannot rely on: it is how they tell a
// draft whose questions cr forced from one whose questions the agent chose.
func (r *draftResult) Text(w *writer) string {
	return "drafted " + w.accent(strconv.Itoa(r.Queued)) + " record(s) for round " +
		strconv.Itoa(r.Round) + " to " + r.Path + "\n" + r.Forced.Disclosure()
}

// newDraftCmd renders the editable draft (§11, §7.1).
//
// The command settles the round's records first and writes once. §9.1's move
// into `queued` and §7.1's rendering are two halves of one answer — a record is
// queued because it was rendered — so a run that stamped the states and then
// failed to write the file would leave findings.ndjson claiming a draft that
// does not exist. Both reach disk under the one lock §2.3.1 requires.
func newDraftCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "draft " + prPlaceholder,
		Short: "Render the editable draft",
		Args:  prArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			// `cr record` gives: §7.1 writes into rounds/<n>/, and
			// a pull request no round has been opened on has no
			// <n> to write into. §11.2 codes that 4.
			round, err := layout.Briefed(owner, repo, pr)
			if err != nil {
				return err
			}
			records, err := roundFindingsOf(layout, owner, repo, pr, round.Round)
			if err != nil {
				return err
			}
			queued, err := queueRecords(records)
			if err != nil {
				return err
			}
			// §6.3.1's second moment, applied over the records
			// this draft holds and before they are rendered: the
			// block a reviewer reads carries the register in its
			// marker, so a forcing applied after the rendering
			// would be a forcing the draft does not show.
			forced := finding.ForceQuestions(queued)
			if err := publishDraft(
				layout, owner, repo, pr, &round, records,
				draft.Render(queued), forced,
			); err != nil {
				return err
			}
			return out.emit(&draftResult{
				Path:   layout.RoundFile(owner, repo, pr, round.Round, state.FileDraft),
				Round:  round.Round,
				Queued: len(queued),
				Forced: forced,
			})
		},
	}
}

// roundFindingsOf reads the records of one round out of findings.ndjson.
//
// The round is part of the question rather than context around it, exactly as
// roundUnitsOf has it: §9.3.5 has a command read only the current round's
// records, and a draft drawn from the whole file would re-render records a
// moved head already moved to `stale`.
func roundFindingsOf(
	l state.Layout, owner, repo string, pr, round int,
) ([]*finding.Finding, error) {
	stored, err := state.ReadRecords[*finding.Finding](l, owner, repo, pr, state.FileFindings)
	if err != nil {
		return nil, err
	}
	current := make([]*finding.Finding, 0, len(stored))
	for _, record := range stored {
		if record.Round == round {
			current = append(current, record)
		}
	}
	return current, nil
}

// queueRecords walks §9.1's `draft` → `queued` row over the round's records and
// returns the ones §7.1 renders.
//
// The move is asked of the transition table rather than assumed, so this
// command writes a state only while §9.1 still has a row allowing it.
//
// A record already in `queued` is rendered again without a transition. §7.1.6
// makes the draft regenerable, so a second run is an ordinary thing to do, and
// §9.1's table has no `queued` → `queued` row for it to ask about — the record
// is where the row already put it. Every other state is left out of the draft
// entirely, which is the whole of what makes the file the set of open records
// rather than the set of records.
func queueRecords(records []*finding.Finding) ([]*finding.Finding, error) {
	queued := make([]*finding.Finding, 0, len(records))
	for _, record := range records {
		switch record.State {
		case finding.StateDraft:
			if err := finding.MayTransition(
				record.ID, finding.Existing(finding.StateDraft),
				finding.StateQueued, finding.ActorDraft,
			); err != nil {
				return nil, err
			}
			record.State = finding.StateQueued
		case finding.StateQueued:
			// Already where the row put it, per §7.1.6.
		default:
			continue
		}
		queued = append(queued, record)
	}
	return queued, nil
}

// publishDraft is the single write, under §2.3.1's lock: the round's records
// carrying the states §9.1 just stamped, the draft they were rendered into, and
// §6.3.2's forcing count in the round summary.
//
// findings.ndjson is replaced for the current round rather than appended to.
// The records being written are the ones just read back out of it, so an append
// would store every one of them a second time; §9.3.5 scopes the replacement to
// this round, so every earlier round's line survives byte for byte.
func publishDraft(
	l state.Layout, owner, repo string, pr int, round *state.Meta,
	records []*finding.Finding, body string, forced finding.Forcings,
) error {
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return err
	}
	stamp := state.Stamp{Head: round.Head, Round: round.Round}
	writes := []func() error{
		func() error { return state.ReplaceStamped(held, state.FileFindings, stamp, records) },
		func() error { return held.WriteRound(round.Round, state.FileDraft, []byte(body)) },
		func() error { return writeForcingCounts(held, round.Round, forced) },
	}
	for _, write := range writes {
		if err := write(); err != nil {
			// The lock is released on the way out of every branch,
			// and the write's own failure is what the caller is
			// told about.
			_ = held.Unlock()
			return err
		}
	}
	return held.Unlock()
}

// summaryForcedToQuestion is §10.3's "forced to question" count, by the key
// summary.json holds it under.
const summaryForcedToQuestion = "forced_to_question"

// writeForcingCounts puts §6.3.2's count per class into the round's
// summary.json, as the one section of that document `cr draft` owns.
//
// §10.3 has `cr merge`, `cr draft` and `cr post` each accumulate their own
// counts into one file, so the write goes through UpdateRoundSection, which
// leaves every field this command does not own byte for byte.
func writeForcingCounts(held *state.Lock, round int, forced finding.Forcings) error {
	return state.UpdateRoundSection(
		held, round, state.FileSummary, summaryForcedToQuestion, forced)
}
