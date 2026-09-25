package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
)

// RuleInputError reports a file handed to `cr rules add` that is not a §2.6
// rule, naming the field (§2.6.3.9).
//
// It wraps the error the loader would have raised over the same bytes found in
// the corpus, because the judgement is the same one and §2.6.3.9 asks for it
// "per §2.6 and §2.6.1.2". What differs is whose file it is. A rule file of
// §2.2's tree that does not parse is §2.6 item 5's exit code 3; a file the
// caller handed a command is input data, which §11.2 codes 1, as it codes a
// malformed line of a file handed to a recording command.
type RuleInputError struct {
	Err error
}

func (e *RuleInputError) Error() string { return e.Err.Error() }

// Unwrap exposes the loader's own error, so the message names the field it
// named.
func (e *RuleInputError) Unwrap() error { return e.Err }

// SourcedFindingError reports a rule that carries `source` and declares `kind:
// finding` (§2.6.3.9). A rule drawn from a colleague's review comments is one
// reviewer's standard until the team adopts it, so it stays in the question
// register.
type SourcedFindingError struct {
	File string
	ID   string
}

func (e *SourcedFindingError) Error() string {
	return fmt.Sprintf("%s: rule %q carries source and kind %q; §2.6.3.9 stores a rule drawn from review "+
		"comments only as kind %q", e.File, e.ID, finding.KindFinding, finding.KindQuestion)
}

// rulesAddResult is what `cr rules add` wrote: the rule, the file, and whether
// it replaced one.
type rulesAddResult struct {
	Repo     string `json:"repo"`
	ID       string `json:"id"`
	Path     string `json:"path"`
	Replaced bool   `json:"replaced"`
}

// Text names the rule and the file it was written to.
func (r *rulesAddResult) Text(w *writer) string {
	verb := "stored"
	if r.Replaced {
		verb = "replaced"
	}
	return fmt.Sprintf("%s: %s rule %s at %s", w.accent(r.Repo), verb, w.accent(r.ID), r.Path)
}

// ruleInputHint is the next actionable step for a rule file that cannot be
// read at all.
const ruleInputHint = "pass the path of the rule file you wrote; §2.6 gives its fields"

// newRulesAddCmd runs §2.6.3.9's `cr rules add --repo <owner/repo> <file>
// [--replace]`: it validates the file as a rule, refuses one the
// per-repository layer already holds unless --replace is given, refuses a
// sourced rule in the finding register, and writes that one file, under a
// lock, to `repos/<owner>/<repo>/rules/<id>.json`.
//
// The bytes written are the bytes handed over. The file is the author's, and
// §2.6.3.3's fence is that cr writes no rule file by itself: rewriting it into
// cr's own serialisation would make the stored rule a thing cr composed.
func newRulesAddCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <file>",
		Short: "Validate and store one rule in the per-repository layer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			replace, err := cmd.Flags().GetBool("replace")
			if err != nil {
				return err
			}
			owner, repo, err := repoOf(cmd)
			if err != nil {
				return err
			}
			body, err := readInput(args[0], ruleInputHint)
			if err != nil {
				return err
			}
			added, err := validatedRule(args[0], body)
			if err != nil {
				return err
			}
			layout, err := state.Default()
			if err != nil {
				return err
			}
			replaced, err := layout.WriteRepoRule(owner, repo, added.ID, body, replace)
			if err != nil {
				return err
			}
			return out.emit(&rulesAddResult{
				Repo: owner + "/" + repo, ID: added.ID,
				Path: layout.RepoRule(owner, repo, added.ID), Replaced: replaced,
			})
		},
	}
	cmd.Flags().Bool("replace", false, "replace a rule of the same id the per-repository layer holds (§2.6.3.9)")
	return cmd
}

// validatedRule holds the handed bytes to §2.6 and §2.6.1.2 through the
// loader's own parse, and to §2.6.3.9's register fence.
//
// The file's own path is what the parse reads the stem from, so §2.6's `id`
// row — equal to the file stem — applies to the file handed over as it will to
// the file written.
func validatedRule(path string, body []byte) (rule.Rule, error) {
	parsed, err := rule.Parse(path, body)
	var malformed *rule.MalformedError
	var invalid *axis.InvalidError
	if errors.As(err, &malformed) || errors.As(err, &invalid) {
		return rule.Rule{}, &RuleInputError{Err: err}
	}
	if err != nil {
		return rule.Rule{}, err
	}
	if len(parsed.Source) > 0 && parsed.Kind == finding.KindFinding {
		return rule.Rule{}, &SourcedFindingError{File: path, ID: parsed.ID}
	}
	return parsed, nil
}
