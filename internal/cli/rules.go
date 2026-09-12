package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
)

// newRulesCmd groups §11's rule commands: the corpus §2.6 resolves, the
// mechanical detection of §2.6.1, and the harvesting of §2.6.3.
func newRulesCmd(out *writer) *cobra.Command {
	cmd := groupCmd("rules", "Inspect, run, and harvest project rules")
	cmd.AddCommand(newRulesListCmd(out), newRulesCheckCmd(out), newRulesSuggestCmd(out))
	return cmd
}

// listedRule is one rule of the effective corpus as `cr rules list` reports
// it: its id and title, the §2.6 item 1 layer it resolved from, and the file
// it is written in, which is where a user prunes it.
type listedRule struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// From is the layer's name. It is not spelled Layer: internal/role
	// fences that identifier to itself, for the reason rule.Source gives.
	From string `json:"layer"`
	Path string `json:"path"`
}

// rulesListResult is §11's `cr rules list`: the effective rules and the layer
// each came from.
//
// The profile is reported because it decides the third layer and §2.6's
// `profiles` scoping, and it is selected from the checkout the command runs
// in exactly as `cr brief` selects it; an empty profile is §2.4.4's outcome.
type rulesListResult struct {
	Repo    string       `json:"repo"`
	Profile string       `json:"profile"`
	Rules   []listedRule `json:"rules"`
	Honesty []string     `json:"honesty"`
}

// Text names the repository and profile, then one line per rule with its layer.
func (r *rulesListResult) Text(w *writer) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s: %d effective rule(s)", w.accent(r.Repo), len(r.Rules))
	writeListedRules(w, &out, r.Rules)
	out.WriteString(w.disclose("\n", "", r.Honesty...))
	return out.String()
}

// rulesDeadResult is §2.6.3.4's report: the rules with no hit and no record
// across the last `rules.dead_after` (pull request, round) pairs of the
// repository's rule ledger.
//
// The window is printed pair by pair because round 8's
// cross-pr-round-ordering-undefined is what defines it, and a reader deciding
// to delete a rule file is owed the rounds that decision rests on.
type rulesDeadResult struct {
	Repo      string             `json:"repo"`
	Profile   string             `json:"profile"`
	DeadAfter int                `json:"dead_after"`
	Window    []rule.LedgerRound `json:"window"`
	Full      bool               `json:"window_full"`
	Dead      []listedRule       `json:"dead"`
	Honesty   []string           `json:"honesty"`
}

// Text names the window, then one line per dead rule with the file to prune.
func (r *rulesDeadResult) Text(w *writer) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s: %d dead rule(s) across the last %d of rules.dead_after=%d round(s)",
		w.accent(r.Repo), len(r.Dead), len(r.Window), r.DeadAfter)
	for _, pair := range r.Window {
		fmt.Fprintf(&out, "\n  pr %d round %d at %s", pair.PR, pair.Round, pair.At.Format("2006-01-02T15:04:05Z"))
	}
	writeListedRules(w, &out, r.Dead)
	out.WriteString(w.disclose("\n", "", r.Honesty...))
	return out.String()
}

// writeListedRules prints one line per rule: id, layer, and file.
func writeListedRules(w *writer, out *strings.Builder, rules []listedRule) {
	for _, listed := range rules {
		fmt.Fprintf(out, "\n  %s  %s  %s", w.accent(listed.ID), listed.From, listed.Path)
	}
}

// newRulesListCmd runs §11's `cr rules list [--dead] --repo <owner/repo>`:
// without the flag, the effective rules and the layer each came from per §2.6
// item 1; with it, §2.6.3.4's rules that produced no hit and no record across
// the last `rules.dead_after` rounds, so the corpus can be pruned rather than
// growing without limit.
//
// It is repository-scoped, like `cr rules suggest`: the ledger holds every pull
// request's rounds, so there is no one round for §9.3 to compare a head
// against. Every read is lock-free per §2.3.2, and it writes nothing.
func newRulesListCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Print the effective rules and the layer each came from",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dead, err := cmd.Flags().GetBool("dead")
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
			listed, deadAfter, err := effectiveRules(layout, owner, repo)
			if err != nil {
				return err
			}
			if !dead {
				return out.emit(listed.result)
			}
			return emitDead(out, layout, owner, repo, listed, deadAfter)
		},
	}
	cmd.Flags().Bool("dead", false, "report rules with no hit or record in the recent rounds (§2.6.3.4)")
	return cmd
}

// effective is the repository's resolved corpus beside the list document that
// reports it.
type effective struct {
	corpus []rule.Resolved
	result *rulesListResult
}

// effectiveRules resolves §2.6 item 1's three layers for the profile §2.4
// selects in the checkout, keeps the rules §2.6's `profiles` row applies under
// it, and returns `rules.dead_after` beside them.
func effectiveRules(l state.Layout, owner, repo string) (*effective, int, error) {
	resolved, err := config.Resolve(config.Sources{
		Environ: os.Environ(), GlobalConfig: l.Config(), RepoConfig: l.RepoConfig(owner, repo),
	})
	if err != nil {
		return nil, 0, err
	}
	dir, err := repoDir()
	if err != nil {
		return nil, 0, err
	}
	selection, err := profile.Select(l.ProfilesDir(), dir, resolved.String("profile"))
	if err != nil {
		return nil, 0, err
	}
	if err := selection.Err(); err != nil {
		return nil, 0, err
	}
	profilePath := ""
	if selection.Selected {
		profilePath = l.Profile(selection.Profile.ID)
	}
	corpus, err := rule.Resolve(l.RepoRulesDir(owner, repo), l.RulesDir(), profilePath, selection.Profile.Rules)
	if err != nil {
		return nil, 0, err
	}
	corpus = rule.ForProfile(corpus, selection.Profile.ID)
	result := &rulesListResult{
		Repo: owner + "/" + repo, Profile: selection.Profile.ID,
		Rules: listedRules(corpus), Honesty: make([]string, 0, 1),
	}
	if !selection.Selected {
		result.Honesty = append(result.Honesty, "no profile matched this checkout (§2.4.4), "+
			"so the profile layer contributed no rule and a rule scoped by `profiles` is left out")
	}
	return &effective{corpus: corpus, result: result}, resolved.Int("rules.dead_after"), nil
}

// emitDead reads the repository's ledger and prints §2.6.3.4's report over the
// effective corpus.
func emitDead(out *writer, l state.Layout, owner, repo string, listed *effective, deadAfter int) error {
	ledger, err := rule.ReadStats(l, owner, repo)
	if err != nil {
		return err
	}
	report := rule.Dead(listed.corpus, ledger, deadAfter)
	honesty := listed.result.Honesty
	if !report.Full {
		honesty = append(honesty, "the rule ledger holds "+strconv.Itoa(len(report.Window))+
			" round(s), fewer than rules.dead_after="+strconv.Itoa(deadAfter)+
			", so no rule can be called dead yet (§2.6.3.4)")
	}
	return out.emit(&rulesDeadResult{
		Repo: listed.result.Repo, Profile: listed.result.Profile, DeadAfter: deadAfter,
		Window: report.Window, Full: report.Full, Dead: listedRules(report.Dead), Honesty: honesty,
	})
}

// listedRules is a corpus in the shape `cr rules list` prints.
func listedRules(corpus []rule.Resolved) []listedRule {
	listed := make([]listedRule, 0, len(corpus))
	for i := range corpus {
		listed = append(listed, listedRule{
			ID: corpus[i].Rule.ID, Title: corpus[i].Rule.Title,
			From: corpus[i].Source.String(), Path: corpus[i].Path,
		})
	}
	return listed
}
