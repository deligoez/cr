package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/state"
)

// reviewResult is what `cr review` reports: §4.6.1's prompts for the round,
// whole, and §4.5.4's report of the lenses that did not run.
//
// The prompts are printed whole rather than counted, because they are the
// command's output in the way §3.7's payload is `cr brief`'s: an agent runs
// them, and a summary would leave it nothing to run.
type reviewResult struct {
	*review.Fanout
}

// Text prints every prompt under a line naming its role and unit, and then the
// lenses that did not run. Nothing is ranked or trimmed: the order is §4.6.1's
// emission order, and every prompt is the text the JSON document carries.
func (r *reviewResult) Text(w *writer) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s round %d at head %s: %s prompt(s)\n",
		w.accent("review"), r.Round, r.Head, strconv.Itoa(len(r.Prompts)))
	for i := range r.Prompts {
		prompt := &r.Prompts[i]
		fmt.Fprintf(&out, "\n%s %s on %s\n\n%s", w.accent("prompt"), prompt.Role, prompt.Unit, prompt.Text)
	}
	for _, entry := range r.Honesty {
		fmt.Fprintf(&out, "\n%s\n", entry)
	}
	return strings.TrimRight(out.String(), "\n")
}

// unknownAxisFlagError reports an `--axis` naming no axis of §1.5.
//
// It is a malformed invocation rather than a malformed file, which is why it is
// not axis.InvalidError: that one names a profile, role, or rule file and
// §11.2 codes it 3, while this is the command line the user just typed. Nothing
// maps it in exit.go, so it takes exitCodeFor's fallback of §11.2's code 2.
type unknownAxisFlagError struct {
	// Value is the flag's value as it was written.
	Value string
}

func (e *unknownAxisFlagError) Error() string {
	return fmt.Sprintf("--axis %q is not an axis id; v0.1 has exactly %s, per §1.5",
		e.Value, strings.Join(axis.IDs(), ", "))
}

// newReviewCmd emits §4.6's fan-out: one prompt per active role and unit.
//
// `cr review` emits the prompts an agent runs, performs no judgement of its
// own, and calls no model (§4.6, invariant 1): the prompts go to the output and
// nothing is sent anywhere. It reads the round `cr brief` recorded, the
// repository it was run from, and the pull request's base, and writes nothing —
// not inside the repository under review (§2.2) and not in the state tree.
//
// `--axis` narrows the fan-out to one axis's roles, which is the control §4.6.5's
// two passes are run through.
func newReviewCmd(out *writer) *cobra.Command {
	var only string
	cmd := &cobra.Command{
		Use:   "review " + prPlaceholder,
		Short: "Emit per-role, per-unit review prompts",
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
			if only != "" && !axis.Valid(only) {
				return &unknownAxisFlagError{Value: only}
			}
			src, err := reviewSources(owner, repo, pr)
			if err != nil {
				return err
			}
			src.Axis = only
			fan, err := review.Run(src)
			if err != nil {
				return err
			}
			return out.emit(&reviewResult{Fanout: fan})
		},
	}
	cmd.Flags().StringVar(&only, "axis", "", "emit only this axis's pass (§4.6.5)")
	return cmd
}

// reviewSources names what one fan-out reads: the state root, the
// configuration of §2.7, the GitHub client, and the repository directory.
func reviewSources(owner, repo string, pr int) (*review.Sources, error) {
	layout, err := state.Default()
	if err != nil {
		return nil, err
	}
	resolved, err := config.Resolve(config.Sources{
		Environ:      os.Environ(),
		GlobalConfig: layout.Config(),
		RepoConfig:   layout.RepoConfig(owner, repo),
	})
	if err != nil {
		return nil, err
	}
	dir, err := repoDir()
	if err != nil {
		return nil, err
	}
	return &review.Sources{
		Layout:  layout,
		GH:      ghClient(),
		Config:  resolved,
		Owner:   owner,
		Repo:    repo,
		PR:      pr,
		RepoDir: dir,
	}, nil
}
