package cli

import (
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
}

// Text names the id, the record answered, and the source. The answer itself is
// what the user just typed, so echoing it would confirm nothing they do not
// already have.
func (r *answerResult) Text(w *writer) string {
	return "recorded " + w.accent(r.Note.ID) + " answering " + r.Note.Record +
		" from " + string(r.Note.Source)
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
			recorded, err := note.Answer(layout, owner, repo, pr, args[1], args[2], parsed, time.Now())
			if err != nil {
				return err
			}
			return out.emit(&answerResult{Note: recorded})
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "where the answer came from: "+sourceList())
	// The error is a flag that does not exist, which the line above rules
	// out.
	_ = cmd.MarkFlagRequired("source")

	return cmd
}
