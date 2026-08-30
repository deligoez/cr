package cli

import (
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// contextResult is what `cr context` has to report: every note the issue's
// store holds, whole.
//
// §3.6.5 asks for the notes *with provenance*, and nothing here summarises that
// away. §3.6.1 fixes what a note carries — the id, the text, the source, the
// pull request it came from, and the timestamp — and §3.6.2 adds the record an
// answer was given to. Every one of them is somewhere a later reader has to go:
// §3.3.2 admits a note as a claim only under the note's id, §3.6.6 retracts one
// by that same id, §8.1.6 discloses the source in the posted body, and the pull
// request and the timestamp are how a reader weighs how old a piece of hearsay
// is and where it was said. So the whole record is printed, and the JSON is the
// stored note rather than a projection of it.
type contextResult struct {
	// IssueKey is the store the notes were read from. It is reported rather
	// than left to the command line, because it is the key every one of
	// these notes is filed under — `cr answer` files under the key a pull
	// request resolved to, which nobody typed.
	IssueKey string `json:"issue_key"`
	// Notes are the store's records in the order they were recorded.
	Notes []note.Note `json:"notes"`
}

// Text renders one line of provenance and one of text per note.
//
// Unlike `cr note`'s rendering, the text is printed: this command's reader did
// not write these notes and may not have seen them, which is the whole reason
// §3.6.5 exists.
func (r *contextResult) Text(w *writer) string {
	if len(r.Notes) == 0 {
		return "no notes recorded against " + w.accent(r.IssueKey)
	}
	// A capacity hint and nothing more: two lines per note plus the
	// heading. Any other number would append to the same slice and print
	// the same text, so no test can tell one from another.
	lines := make([]string, 0, 2*len(r.Notes)+1)
	lines = append(lines, w.accent(r.IssueKey)+": "+strconv.Itoa(len(r.Notes))+" note(s)")
	for i := range r.Notes {
		lines = append(lines,
			"  "+w.accent(r.Notes[i].ID)+" "+provenanceOf(&r.Notes[i]),
			"    "+r.Notes[i].Text,
		)
	}
	return strings.Join(lines, "\n")
}

// provenanceOf renders where one note came from: its source, the pull request
// it was recorded from, when it was recorded, and the record it answers when it
// answers one (§3.6.1, §3.6.2).
//
// The timestamp is rendered in RFC 3339 because that is what the store holds —
// note.Note stamps in UTC so two notes recorded on different machines order the
// same everywhere, and a rendering in the reader's local zone would give that
// away for nothing.
func provenanceOf(n *note.Note) string {
	provenance := "from " + string(n.Source) +
		" on pr " + strconv.Itoa(n.PR) +
		" at " + n.RecordedAt.Format(time.RFC3339)
	if n.Record != "" {
		provenance += ", answering " + n.Record
	}
	return provenance
}

// newContextCmd prints the accumulated notes for one issue key (§3.6.5).
//
// It is addressed by issue key and takes neither a pull request nor a round,
// which is §3.6.4 expressed as a command surface rather than asserted in a
// comment: the store is keyed by the issue, so what one round and one pull
// request sees is what every later round and every later pull request resolving
// to the same key sees. There is nothing here to filter by, and a `--pr` flag
// would be a way of not seeing a note somebody recorded.
//
// The read takes no lock, per §2.3.2, and this command writes nothing at all —
// not even the store's own directory, which is why it reaches the file through
// state.ReadContextRecords rather than through the lock `cr note` writes under.
func newContextCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "context <ISSUE-KEY>",
		Short: "Print the accumulated notes for an issue key",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			layout, err := state.Default()
			if err != nil {
				return err
			}
			notes, err := note.Load(layout, args[0])
			if err != nil {
				return err
			}
			return out.emit(&contextResult{IssueKey: args[0], Notes: notes})
		},
	}
}
