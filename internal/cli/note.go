package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// noteResult is what `cr note` has to report: the note it appended, whole.
// §3.6.6 makes a note revocable by id, so the id has to reach the person who
// will one day retract it, and §8.1.6 will disclose the source, so both are
// printed rather than only confirmed.
type noteResult struct {
	// Note is the record as it was written to the store.
	Note note.Note `json:"note"`
}

// Text names the id and the source. The text itself is what the user just
// typed, so echoing it would confirm nothing they do not already have.
func (r *noteResult) Text(w *writer) string {
	return "recorded " + w.accent(r.Note.ID) + " from " + string(r.Note.Source)
}

// newNoteCmd stores one out-of-band fact against an issue key (§3.6.1).
//
// The store is keyed by issue and not by pull request, so this command takes no
// `<pr>` positional: §3.6.4 has the note load for every later round and every
// later pull request that resolves to the same key. `--pr` is provenance, which
// is a different thing from scope, and it is required because §3.6.1 requires
// the note to carry the pull request it came from and cr forms no opinion about
// which one that was.
//
// §11's table gives `cr note` a second form, `--remove <note-id>`, which is not
// v0.1's here: retraction has its own task, and it reaches this command as a
// flag alongside these, not as a rewrite of them.
func newNoteCmd(out *writer) *cobra.Command {
	var source string
	var pr int

	cmd := &cobra.Command{
		Use:   "note <ISSUE-KEY> <text>",
		Short: "Store an out-of-band fact against an issue key",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			parsed, err := note.ParseSource(source)
			if err != nil {
				return err
			}
			layout, err := state.Default()
			if err != nil {
				return err
			}
			recorded, err := note.Append(layout, args[0], args[1], parsed, pr, time.Now())
			if err != nil {
				return err
			}
			return out.emit(&noteResult{Note: recorded})
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "where the fact came from: "+sourceList())
	cmd.Flags().IntVar(&pr, "pr", 0, "the pull request the fact came from")
	// The error is a flag that does not exist, which the two lines above
	// rule out.
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("pr")

	return cmd
}

// sourceList renders §3.6.3's set for the flag's help, read out of the package
// that owns the set so the two cannot drift apart.
func sourceList() string {
	admitted := make([]string, 0)
	for _, source := range note.Sources() {
		admitted = append(admitted, string(source))
	}
	return strings.Join(admitted, ", ")
}
