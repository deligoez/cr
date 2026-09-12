package cli

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// statsResult is §7.3.2's report: the five counts per class and per rule, over
// the repository's whole triage ledger.
//
// The repository is reported rather than left to the command line for the
// reason `cr context` reports its issue key: a document that names only counts
// is a document a reader cannot tell one repository's from another's once it
// has been saved or piped somewhere.
type statsResult struct {
	// Repo is the repository the ledger belongs to, as `<owner>/<repo>`.
	Repo string `json:"repo"`
	// Events is how many triage events the counts were computed over,
	// which is what tells a repository with no review history from one
	// whose classes all happen to have been raised once.
	Events int `json:"events"`
	finding.TriageReport
}

// Text names what was counted and then one line per class and per rule.
//
// The two cuts are printed under their own headings rather than interleaved,
// because §7.3.2 asks for both and a class id and a rule id are different
// namespaces: `unchecked-error` the class and `no-dropped-error` the rule can
// count the same events, and a reader has to be able to tell which of the two
// a line is about.
func (r *statsResult) Text(w *writer) string {
	var out strings.Builder
	out.WriteString(w.accent(r.Repo) + ": " + strconv.Itoa(r.Events) +
		" triage event(s) over " + strconv.Itoa(len(r.Classes)) + " class(es)")
	out.WriteString("\nper class (raised/kept/softened/not-here/wrong)")
	if len(r.Classes) == 0 {
		out.WriteString("\n  none")
	}
	for _, class := range r.Classes {
		out.WriteString("\n  " + w.accent(class.Class) + " " + countsLine(class.TriageCounts))
	}
	out.WriteString("\nper rule (raised/kept/softened/not-here/wrong)")
	if len(r.Rules) == 0 {
		out.WriteString("\n  none")
	}
	for _, rule := range r.Rules {
		out.WriteString("\n  " + w.accent(rule.Rule) + " " + countsLine(rule.TriageCounts))
	}
	return out.String()
}

// countsLine renders §7.3.2's five counts in the order the section names them,
// which is also the order the heading above spells out.
func countsLine(counts finding.TriageCounts) string {
	return strings.Join([]string{
		strconv.Itoa(counts.Raised),
		strconv.Itoa(counts.Kept),
		strconv.Itoa(counts.Softened),
		strconv.Itoa(counts.DiscardedNotHere),
		strconv.Itoa(counts.DiscardedWrong),
	}, "/")
}

// newStatsCmd runs §11's `cr stats --repo <owner/repo>`: §7.3.2's per class and
// per rule counts over the repository's `triage.ndjson`.
//
// It is scoped to the repository and not to a pull request, because §7.3.4
// computes the demotion rate over that repository's whole ledger — a rate over
// one pull request's events would be a number with no sample behind it.
// `--repo` is §11.1's global flag, registered on the root command.
//
// §9.3 therefore has nothing to say about it, for the reason `cr rules suggest`
// is exempt: there is no one round whose head §9.3.1 could compare against a
// current one, and no per-PR state for §9.3.2 to refuse the write of. §9.3.5's
// round scoping does not reach it either — triage.ndjson is repository state
// under §2.2 and not one of §2.3.3's eight per-PR files, so there is no round
// stamped on a record here for a read to be narrowed to.
//
// The read takes no lock, per §2.3.2, and this command writes nothing at all:
// §7.3.7 makes both candidacies reports to the user, so there is no flag here
// that would have cr act on one.
func newStatsCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Report triage statistics, demotion and volume candidates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			owner, repo, err := repoOf(cmd)
			if err != nil {
				return err
			}
			layout, err := state.Default()
			if err != nil {
				return err
			}
			events, err := finding.TriageEvents(layout, owner, repo)
			if err != nil {
				return err
			}
			report, err := finding.Tally(events)
			if err != nil {
				return err
			}
			return out.emit(&statsResult{
				Repo: owner + "/" + repo, Events: len(events), TriageReport: report,
			})
		},
	}
}
