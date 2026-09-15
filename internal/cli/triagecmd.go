package cli

import (
	"errors"
	"io"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/state"
)

// triageResult is what `cr triage` has to report: §7.2.4's record id and verb,
// whether `--body-file` replaced the block's agent region, and the draft the
// edit was made in.
type triageResult struct {
	// ID is the record whose block was edited.
	ID string `json:"id"`
	// Verb is the §7.2.4 verb applied to it.
	Verb draft.Verb `json:"verb"`
	// BodyReplaced reports whether the block's agent region now holds the
	// content `--body-file` named.
	BodyReplaced bool `json:"body_replaced"`
	// Path is the rounds/<n>/draft.md the edit was made in.
	Path string `json:"path"`
}

// Text names the record, the verb, and the draft, which is the file the next
// `cr draft` or `cr post` reads the edit back out of.
func (r *triageResult) Text(w *writer) string {
	text := w.accent(r.ID) + ": " + string(r.Verb)
	if r.BodyReplaced {
		text += ", body replaced"
	}
	return text + " in " + r.Path
}

// triageBodyFlag is §7.2.4's flag naming the content that replaces a block's
// agent region.
const triageBodyFlag = "body-file"

// errBodyWithDiscard is §7.2.4's usage refusal of `--body-file` beside a verb
// that discards the block. It is left unmapped, so §11.2 codes it 2.
var errBodyWithDiscard = errors.New(
	"--body-file replaces the agent region of a block the verb keeps, and §7.2.4 refuses it " +
		"with not-here or wrong, which discard the block")

// newTriageCmd applies one triage verb to the draft (§11, §7.2.4).
//
// It is the draft edit and nothing more. §7.2 has triage happen by editing the
// file or through this command with the same effect, so the command writes the
// bytes the matching hand edit writes and leaves every consequence — the
// discard, the waiver, the triage event, §8.1.3's refusal of a body — to the
// `cr draft` or `cr post` that reads the file back. It writes per-PR state, so
// §9.3.2 refuses it on a moved head exactly as it refuses `cr draft`.
func newTriageCmd(out *writer) *cobra.Command {
	var bodyFile string
	cmd := &cobra.Command{
		Use:   "triage " + prPlaceholder + " <record-id> not-here|wrong|soften|keep",
		Short: "Apply one triage verb to the draft",
		Args:  prArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			pr, err := parsePR(args[0])
			if err != nil {
				return err
			}
			verb, err := draft.ParseVerb(args[2])
			if err != nil {
				return err
			}
			replacing := cmd.Flags().Changed(triageBodyFlag)
			if replacing && !verb.KeepsBlock() {
				return errBodyWithDiscard
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
			// §9.3.2: this command writes the round's draft.md.
			if err := round.RefuseStale(); err != nil {
				return err
			}
			var body *string
			if replacing {
				read, err := triageBody(cmd, bodyFile)
				if err != nil {
					return err
				}
				body = &read
			}
			if err := triageDraft(layout, owner, repo, pr, round.Round, args[1], verb, body); err != nil {
				return err
			}
			return out.emit(&triageResult{
				ID: args[1], Verb: verb, BodyReplaced: replacing,
				Path: layout.RoundFile(owner, repo, pr, round.Round, state.FileDraft),
			})
		},
	}
	cmd.Flags().StringVar(&bodyFile, triageBodyFlag, "",
		"replace the block's agent region with this file's content, or standard input's for -")
	return cmd
}

// triageBody reads `--body-file`: the named file, or standard input for `-`.
func triageBody(cmd *cobra.Command, path string) (string, error) {
	const hint = "§7.2.4 replaces the block's agent region with this file's content"
	if path != "-" {
		read, err := readInput(path, hint)
		return string(read), err
	}
	read, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return "", state.FileFailure("read", "standard input", hint, err)
	}
	return string(read), nil
}

// triageDraft is the edit under §2.3.1's per-PR lock: the round's draft.md read,
// the verb applied, and the result written back, so a second `cr triage` or a
// `cr draft` cannot interleave with it. A refused edit writes nothing.
func triageDraft(
	l state.Layout, owner, repo string, pr, round int, id string, verb draft.Verb, body *string,
) error {
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return err
	}
	file, err := l.ReadRound(owner, repo, pr, round, state.FileDraft)
	if err == nil {
		var edited string
		if edited, err = draft.Triaged(string(file), id, verb, body); err == nil {
			err = held.WriteRound(round, state.FileDraft, []byte(edited))
		}
	}
	if err != nil {
		_ = held.Unlock()
		return err
	}
	return held.Unlock()
}
