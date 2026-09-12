package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// rulesCheckResult is what `cr rules check` reports: §2.6.1.1's hits over the
// round's diff, and §4.3.6's placement of each on the unit that contains it.
//
// It carries hits and nothing a verdict would need, which is §2.6.1.5's first
// sentence given a shape. There is no kind, severity, or grade here, and no
// record: a rule declaring `kind: finding` keeps that default on the rule, and
// it becomes a record's only when the agent writes the record and `cr record`
// grades it. Confirmation is what a rule cannot supply for itself, so a detect
// block alone has no field through which to buy §6.3's assertion register.
type rulesCheckResult struct {
	// Round and Head are the round the diff was read at.
	Round int    `json:"round"`
	Head  string `json:"head"`
	// Hits are every hit of the round, in corpus order and then diff order.
	Hits []rule.Hit `json:"hits"`
	// Units are §4.3.6's attachments, one per unit of the round.
	Units []rule.Attachment `json:"units"`
	// Honesty carries §9.3.1's comparison of the round's head against the
	// pull request's current one, rendered as the sentence §11.1 exempts
	// from `--quiet`.
	//
	// This command discloses where the writers of §9.3.2 refuse. What it
	// writes is §2.6.1.6's ledger, which §2.2 scopes to the repository
	// rather than to the pull request, so the refusal does not reach it —
	// and a reader is owed the comparison either way, because the hits
	// below were evaluated over the diff at the round's head.
	Honesty []string `json:"honesty"`
}

// Text names how many hits the round's diff holds and what a hit is not, then
// each hit by rule and location. The line of text is included only in the JSON
// document: a terminal reader has the diff already.
func (r *rulesCheckResult) Text(w *writer) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s rule hits at %s in round %d. Detection reports hits, never verdicts "+
		"(§2.6.1.5): a hit reaches a draft only as a record the agent writes naming the rule",
		w.accent(strconv.Itoa(len(r.Hits))), r.Head, r.Round)
	for _, hit := range r.Hits {
		fmt.Fprintf(&out, "\n  %s at %s:%d", hit.RuleID, hit.Path, hit.Line)
	}
	out.WriteString(w.disclose("\n", "", r.Honesty...))
	return out.String()
}

// newRulesCheckCmd runs §11's `cr rules check <pr>`, §2.6.1.1's mechanical
// evaluation over the added and modified RIGHT-side lines of the round's diff.
//
// It reports hits and never verdicts, per §2.6.1.5: a hit reaches a draft only
// when the agent confirms it, and confirmation is what a rule cannot supply for
// itself. That is why the command takes no severity, kind, or grade of its own
// and writes no record — there is no judgement here for a flag to influence,
// and findings.ndjson is `cr record`'s to write from the agent's own file. What
// it does write is §2.6.1.6's hit entries, to the repository's rule ledger. A
// hit the agent never writes a record for is dropped by that absence: nothing
// in cr turns a hit into a record, so there is no path by which one could reach
// the draft unconfirmed.
//
// The round is the one `cr brief` recorded, read through state.Layout.Briefed,
// so the diff is taken at the head the round's units were formed from.
func newRulesCheckCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "check " + prPlaceholder,
		Short: "Run mechanical rule detection over the diff",
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
			layout, err := state.Default()
			if err != nil {
				return err
			}
			round, err := briefedRound(layout, owner, repo, pr)
			if err != nil {
				return err
			}
			checked, err := detectRound(layout, owner, repo, pr, &round)
			if err != nil {
				return err
			}
			// §2.6.1.6: every hit reaches the repository's ledger,
			// keyed so a second run of the same round overwrites
			// rather than counts it again.
			if err := rule.RecordHits(layout, owner, repo, checked.Hits, &rule.Occasion{
				PR: pr, Round: round.Round, Head: round.Head, At: time.Now(),
			}); err != nil {
				return err
			}
			return out.emit(checked)
		},
	}
}

// detectRound evaluates the round's rule corpus over the round's diff and
// places every hit on the unit that contains it.
//
// The corpus is resolved and compiled before the diff is read, so §2.6.5's and
// §2.6.1.2's aborts reach the caller before anything outside the state tree is
// asked a question.
func detectRound(
	l state.Layout, owner, repo string, pr int, round *state.Round,
) (*rulesCheckResult, error) {
	matchers, err := roundMatchers(l, owner, repo, round.ProfileID)
	if err != nil {
		return nil, err
	}
	hunks, err := roundHunks(owner, repo, pr, round.Head)
	if err != nil {
		return nil, err
	}
	formed, err := roundUnitsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return nil, err
	}
	units := make([]unit.Unit, 0, len(formed))
	for i := range formed {
		units = append(units, formed[i].Unit)
	}
	hits := rule.Evaluate(matchers, hunks)
	return &rulesCheckResult{
		Round: round.Round, Head: round.Head, Hits: hits, Units: rule.Attach(units, hits),
		Honesty: []string{round.Disclosure()},
	}, nil
}

// roundMatchers compiles every detect block of the round's rule corpus that
// applies under the round's profile, per §2.6's `profiles` row, which is the
// same rule.ForProfile `cr review` asks.
func roundMatchers(l state.Layout, owner, repo, profileID string) ([]rule.Matcher, error) {
	corpus, err := roundCorpus(l, owner, repo, profileID)
	if err != nil {
		return nil, err
	}
	return rule.Compile(rule.ForProfile(corpus, profileID))
}

// roundCorpus resolves §2.6 item 1's three layers for the round's profile: the
// per-repository rules, the global ones, and the profile's own array.
func roundCorpus(l state.Layout, owner, repo, profileID string) ([]rule.Resolved, error) {
	resolved, file, err := roundProfile(l, profileID)
	if err != nil {
		return nil, err
	}
	return rule.Resolve(l.RepoRulesDir(owner, repo), l.RulesDir(), file, resolved.Rules)
}

// roundHunks takes §3.4.1's diff at the round's head, against the merge base
// with the base GitHub reports for the pull request, and parses its hunks.
func roundHunks(owner, repo string, pr int, head string) ([]git.Hunk, error) {
	dir, err := repoDir()
	if err != nil {
		return nil, err
	}
	opened, err := ghClient().PullRequest(owner, repo, pr)
	if err != nil {
		return nil, err
	}
	diff, err := git.DiffAgainstMergeBase(dir, opened.Base, head)
	if err != nil {
		return nil, err
	}
	return git.ParseHunks(diff.Patch)
}
