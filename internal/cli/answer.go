package cli

import (
	"errors"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// answerResult is what `cr answer` has to report: the note it appended, whole.
// It is the same payload `cr note` returns, because §3.6.2 writes the same kind
// of record into the same store — one field further on, naming the record the
// answer was given to.
type answerResult struct {
	// Note is the record as it was written to the store.
	Note note.Note `json:"note"`
	// Honesty carries §9.3.1's comparison of the round's head against the
	// pull request's current one, rendered as the sentence §11.1 exempts
	// from `--quiet`. It is empty, and never nil, when no round has been
	// opened and there is therefore no recorded head to compare against.
	Honesty []string `json:"honesty"`
}

// Text names the id, the record answered, and the source. The answer itself is
// what the user just typed, so echoing it would confirm nothing they do not
// already have.
func (r *answerResult) Text(w *writer) string {
	text := "recorded " + w.accent(r.Note.ID) + " answering " + r.Note.Record +
		" from " + string(r.Note.Source)
	return text + w.disclose("\n", "", r.Honesty...)
}

// newAnswerCmd stores the answer to a posted question as a note (§3.6.2).
//
// It is addressed by pull request where `cr note` is addressed by issue key,
// and §11 says why: a record id is scoped to a pull request, so every command
// naming one takes the pull request. The issue key the note is filed under is
// read from that pull request's §2.3 metadata, so the answer lands under the
// key the round resolved rather than one retyped here.
//
// The pull request therefore serves twice — the scope of the record id, and the
// provenance §3.6.1 requires of every note — which is why this command takes no
// `--pr` flag while `cr note` requires one.
func newAnswerCmd(out *writer) *cobra.Command {
	var source string

	cmd := &cobra.Command{
		Use:   "answer " + prPlaceholder + " <record-id> <text>",
		Short: "Store the answer to a posted question as a note",
		Args:  prArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			pr, err := parsePR(args[0])
			if err != nil {
				return err
			}
			parsed, err := note.ParseSource(source)
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
			honesty, err := answerHonesty(layout, owner, repo, pr)
			if err != nil {
				return err
			}
			recorded, err := note.Answer(layout, owner, repo, pr, args[1], args[2], parsed, time.Now())
			if err != nil {
				return err
			}
			return out.emit(&answerResult{Note: recorded, Honesty: honesty})
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "where the answer came from: "+sourceList())
	// The error is a flag that does not exist, which the line above rules
	// out.
	_ = cmd.MarkFlagRequired("source")

	return cmd
}

// answerHonesty is §9.3.1's comparison for `cr answer`, which reads meta.json
// for the issue key and is therefore bound by it.
//
// It runs before the note is written and gates nothing. §9.3.2's refusal binds
// the commands that write per-PR state, and §3.6's context store is not one:
// §9.3.5 exempts it from round scoping outright, so an answer arriving after
// the head moved is still a true fact about a question that was really asked.
//
// A pull request no round has been opened on is the one case with nothing to
// disclose — §9.3.1 compares against a recorded head and round 0 has none — so
// that refusal alone is read as "no comparison" and note.Answer below says what
// the absence means. Every other failure is returned, including a head cr could
// not read: §9.3.1 leaves no way to skip the comparison, and reporting a round
// as current because nobody looked is the outcome it exists to prevent.
func answerHonesty(l state.Layout, owner, repo string, pr int) ([]string, error) {
	honesty := make([]string, 0, 1)
	round, err := briefedRound(l, owner, repo, pr)
	var unbriefed *state.NotBriefedError
	if errors.As(err, &unbriefed) {
		return honesty, nil
	}
	if err != nil {
		return nil, err
	}
	return append(honesty, round.Disclosure()), nil
}
