package cli

import (
	"fmt"
	"os"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/proposal"
	"github.com/deligoez/cr/internal/state"
)

// proposalReport is §10.1.8: the round's §5.7 proposals by state.
//
// The open ones are named and not only counted. An open proposal is an ask
// nobody has answered, and the reader's next move is to run one — which needs
// its id.
type proposalReport struct {
	// Total is every proposal the round holds.
	Total int `json:"total"`
	// Open, Run and Unrunnable are §5.7's three states.
	Open       int `json:"open"`
	Run        int `json:"run"`
	Unrunnable int `json:"unrunnable"`
	// OpenIDs are the open proposals' ids, in file order.
	OpenIDs []string `json:"open_ids"`
	// Unrunnables are the ones §5.7.5 stored with a reason, each named
	// with it: a proposal cr cannot run is a role's ask that will never be
	// answered, and a count alone would leave the reader looking for it.
	Unrunnables []unrunnableProposal `json:"unrunnables"`
}

// unrunnableProposal is one §5.7.5 proposal and why it cannot run.
type unrunnableProposal struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// statusSaid is §10.1.3's report and the lines §11.1 exempts from `--quiet`,
// with §5.7.6's disclosure of the round's proposals after them.
//
// The order is the sections' own: the lenses that did not run come before what
// the round has asked for and cannot spend.
func statusSaid(
	l state.Layout, owner, repo string, pr int, round *state.Round,
	lenses coverage.Lenses, records []*finding.Finding, verdict coverage.Completeness,
	proposals []string,
) ([]string, error) {
	said, err := statusHonesty(l, owner, repo, pr, round, lenses, records, verdict)
	if err != nil {
		return nil, err
	}
	return append(said, proposals...), nil
}

// statusProposals is §10.1.8 as `cr status` needs it: the report, and §5.7.6's
// disclosure when the round has asked for more experiments than its cap still
// allows.
func statusProposals(
	l state.Layout, owner, repo string, pr int, round *state.Round, spent int,
) (proposalReport, []string, error) {
	report, err := proposalsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return report, nil, err
	}
	disclosed := report.overCap(l, owner, repo, spent)
	return report, disclosed, nil
}

// proposalsOf counts the round's proposals by state and names the open ones.
func proposalsOf(l state.Layout, owner, repo string, pr, round int) (proposalReport, error) {
	report := proposalReport{
		OpenIDs:     make([]string, 0),
		Unrunnables: make([]unrunnableProposal, 0),
	}
	stored, err := state.ReadStamped[proposal.Proposal](l, owner, repo, pr, state.FileProposals, round)
	if err != nil {
		return report, err
	}
	report.Total = len(stored)
	for i := range stored {
		switch stored[i].State {
		case proposal.StateRun:
			report.Run++
		case proposal.StateUnrunnable:
			report.Unrunnable++
			report.Unrunnables = append(report.Unrunnables,
				unrunnableProposal{ID: stored[i].ID, Reason: stored[i].Reason})
		default:
			report.Open++
			report.OpenIDs = append(report.OpenIDs, stored[i].ID)
		}
	}
	return report, nil
}

// overCap is §5.7.6's disclosure: the round's open proposals outnumber the
// probe executions `probe.max_per_round` still allows.
//
// It is said only where it is true, as §5.6.4's own cap notice is. A reader
// told on every run how many probes are left learns nothing; a reader told that
// the round has asked for more experiments than it can still run learns that
// some of them will not happen unless the cap is raised.
//
// spent is how many probes the pull request has recorded for the round.
func (r *proposalReport) overCap(l state.Layout, owner, repo string, spent int) []string {
	if r.Open == 0 {
		return nil
	}
	resolved, err := config.Resolve(config.Sources{
		Environ:      os.Environ(),
		GlobalConfig: l.Config(),
		RepoConfig:   l.RepoConfig(owner, repo),
	})
	if err != nil {
		// The cap is a disclosure, not a verdict. A configuration cr
		// cannot resolve fails the commands that need it, and saying
		// nothing here is the honest answer rather than a guess at the
		// default.
		return nil
	}
	capped := resolved.Int("probe.max_per_round")
	left := max(capped-spent, 0)
	if r.Open <= left {
		return nil
	}
	return []string{fmt.Sprintf(
		"§5.7.6: the round holds %d open proposal(s) and probe.max_per_round leaves %d execution(s) "+
			"of its %d, so not every experiment the roles asked for can be run in this round",
		r.Open, left, capped)}
}
