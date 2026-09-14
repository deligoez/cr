package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/intent"
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

// retractResult is what `cr note --remove` has to report: the note as it now
// stands, and what a citation of it may now do (§3.6.6).
type retractResult struct {
	// Note is the record as it now sits on disk, carrying its retraction.
	// It is printed whole for the reason contextResult prints one whole:
	// the text is how the reader recognises the records that rested on it.
	Note note.Note `json:"note"`
	// Standing is what a record or coverage cell citing this note may now
	// do. It is reported rather than left to be inferred from the
	// timestamp, because it is the one value §6.3's register and §8.1.6's
	// provenance region both turn on.
	Standing note.Standing `json:"standing"`
}

// Text names the id and says plainly what the retraction did to whatever rested
// on it. §3.6.6 asks for the dependants to be reported rather than silently
// retained, and this is that report reaching the person who just retracted it.
func (r *retractResult) Text(w *writer) string {
	return "retracted " + w.accent(r.Note.ID) +
		": every record and coverage cell citing it needs re-evaluation, and none may assert on it"
}

// newNoteCmd stores one out-of-band fact against an issue key (§3.6.1), and
// retracts one through `--remove` (§3.6.6).
//
// The store is keyed by issue and not by pull request, so this command takes no
// `<pr>` positional: §3.6.4 has the note load for every later round and every
// later pull request that resolves to the same key. `--pr` is provenance, which
// is a different thing from scope, and it is required because §3.6.1 requires
// the note to carry the pull request it came from and cr forms no opinion about
// which one that was.
//
// `--remove` is §11's second row for this command, and it takes no positional
// at all: §3.6.1 forms an id as `<ISSUE-KEY>#n<n>`, so the id already names the
// store it belongs to.
func newNoteCmd(out *writer) *cobra.Command {
	var source string
	var pr int
	var remove string

	cmd := &cobra.Command{
		Use:   "note <ISSUE-KEY> <text>",
		Short: "Store an out-of-band fact against an issue key",
		Args: func(cmd *cobra.Command, args []string) error {
			if retracting(cmd) {
				// Not cobra.NoArgs, which reports a stray positional as
				// an unknown command of a command that has none.
				if len(args) > 0 {
					return fmt.Errorf("unexpected argument %q: `cr note --remove <note-id>` takes "+
						"the note id as the flag's value and no argument", args[0])
				}
				return nil
			}
			return cobra.ExactArgs(2)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if retracting(cmd) {
				layout, err := state.Default()
				if err != nil {
					return err
				}
				retracted, err := note.Retract(layout, remove, time.Now())
				if err != nil {
					return err
				}
				return out.emit(&retractResult{
					Note: retracted, Standing: retracted.Standing(),
				})
			}
			parsed, err := note.ParseSource(source)
			if err != nil {
				return err
			}
			// Asked of the flag rather than its value, so an absent `--pr`
			// is not reported as a pull request 0 nobody typed.
			if !cmd.Flags().Changed("pr") {
				return errors.New("--pr is required: §3.6.1 records the pull request a note came from, " +
					"so name it, e.g. --pr 42")
			}
			layout, err := state.Default()
			if err != nil {
				return err
			}
			if err := checkNoteKey(cmd, layout, args[0]); err != nil {
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
	cmd.Flags().StringVar(&remove, "remove", "", "retract the note with this id (§3.6.6)")
	// §3.6.1's two flags are required of an append and meaningless to a
	// retraction, which is a different row of §11's table reached through
	// the same command. cobra's required flags are unconditional, so the
	// requirement is kept where the append is made instead — ParseSource
	// refuses an absent `--source`, the append path an absent `--pr`, and
	// note.Append a `--pr` below 1, all through §11.2's code 2, which is
	// where cobra put them too. Naming
	// them as exclusions here means `--remove` with either is refused
	// rather than quietly ignoring one.
	cmd.MarkFlagsMutuallyExclusive("remove", "source")
	cmd.MarkFlagsMutuallyExclusive("remove", "pr")

	return cmd
}

// checkNoteKey refuses an issue key the effective `intent.key_pattern` does not
// match whole (§3.2), before anything reaches the store the key would name.
//
// The per-repository layer is consulted the way `cr config` consults it: through
// repoOf when a repository is named or detected, and left out when detection
// names none, because a note is keyed by issue and may be typed anywhere.
func checkNoteKey(cmd *cobra.Command, l state.Layout, issueKey string) error {
	sources := config.Sources{Environ: os.Environ(), GlobalConfig: l.Config()}
	owner, name, err := repoOf(cmd)
	var undetected *RepositoryDetectionError
	switch {
	case errors.As(err, &undetected):
	case err != nil:
		return err
	default:
		sources.RepoConfig = l.RepoConfig(owner, name)
	}
	resolved, err := config.Resolve(sources)
	if err != nil {
		return err
	}
	return intent.CheckKey(issueKey, resolved.String(intentKeyPattern))
}

// retracting reports whether this run is §11's `cr note --remove <note-id>`
// row rather than its `cr note <ISSUE-KEY> <text>` one.
//
// It reads whether the flag was given rather than whether it carries a value,
// so `--remove ""` is a retraction of an empty id — refused as an id no note
// bears — rather than silently becoming an append that was never asked for.
func retracting(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("remove")
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
