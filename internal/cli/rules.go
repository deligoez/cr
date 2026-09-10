package cli

import (
	"github.com/spf13/cobra"
)

// newRulesCmd groups §11's rule commands: the corpus §2.6 resolves, the
// mechanical detection of §2.6.1, and the harvesting of §2.6.3.
func newRulesCmd(out *writer) *cobra.Command {
	cmd := groupCmd("rules", "Inspect, run, and harvest project rules")
	cmd.AddCommand(newRulesListCmd(), newRulesCheckCmd(out), newRulesSuggestCmd())
	return cmd
}

// newRulesListCmd registers §11's `cr rules list [--dead] --repo <owner/repo>`:
// without the flag, the effective rules and the layer each came from per §2.6
// item 1; with it, §2.6.3.4's rules that produced no hit and no record across
// the last `rules.dead_after` rounds, so the corpus can be pruned rather than
// growing without limit.
//
// The behaviour belongs to rules-list-dead, which owns both forms; what is
// registered here is the shape.
func newRulesListCmd() *cobra.Command {
	cmd := stubCmd("list", "Print the effective rules and the layer each came from", cobra.NoArgs)
	cmd.Flags().Bool("dead", false, "report rules with no hit or record in the recent rounds (§2.6.3.4)")
	return cmd
}

// newRulesSuggestCmd registers §11's `cr rules suggest --repo <owner/repo>`,
// §2.6.3.1's scan of the bodies of comments posted from recorded rounds,
// grouped by class and by normalised body per §1.4.
//
// §2.6.3.3 makes it a report and nothing more: cr must not write a rule file by
// itself, so there is no flag here that would have it do so.
//
// The behaviour belongs to rules-suggest-command and rules-suggest-reports-only;
// what is registered here is the shape.
func newRulesSuggestCmd() *cobra.Command {
	return stubCmd(
		"suggest",
		"Propose rules from recurring comment history",
		cobra.NoArgs,
	)
}
