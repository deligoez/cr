package cli

import (
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/config"
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
	// FirstSeen is §7.3.3's report: every class with the pull request and
	// round the ledger first held it on.
	//
	// It is here rather than as a flag, because §7.3.3 asks for the drift
	// to be visible and a report nobody runs discloses nothing. A reader
	// scanning it sees `unchecked-error` first seen on the repository's
	// first reviewed pull request and `dropped-error` first seen forty
	// rounds later, which is the comparison the section exists to make
	// possible.
	FirstSeen []finding.FirstSeen `json:"first_seen"`
	// Threshold and MinSamples are the `stats` settings the candidacies
	// below were decided on.
	//
	// They are reported for the reason `cr rules suggest` reports its
	// harvest minimum: nothing else in the document explains an empty
	// list, and a run that found no candidate and a run whose threshold
	// somebody raised to 0.95 print the same empty list otherwise.
	Threshold  float64 `json:"demote_threshold"`
	MinSamples int     `json:"min_samples"`
	// Demotion are §7.3.4's demotion candidates, and Bound is §7.3.5's
	// sentence about what their rate is and is not.
	//
	// The sentence is a field rather than prose in the terminal rendering
	// alone, because an agent reads this document and §7.3.5 forbids the
	// rate to be presented as a measured precision to anyone. The rate's
	// own key says the same thing a second time, so a reader who prints
	// one number without its document still cannot lose the caveat.
	Demotion []finding.DemotionCandidate `json:"demotion_candidates"`
	Bound    string                      `json:"demotion_rate_bound"`
	// Volume are §7.3.6's volume candidates, reported separately from the
	// demotion ones because the remedy differs: a class here is accurate
	// and merely rarely worth posting, so softening it would turn a true
	// finding nobody wanted into a question nobody wanted.
	//
	// A class can appear on both lists, on one, or on neither, and the
	// document says which rather than ranking them: §7.3.7 makes both
	// candidacies reports to the user, and a single ordered list would be
	// cr deciding which fault matters more.
	Volume []finding.VolumeCandidate `json:"volume_candidates"`
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
	out.WriteString("\nfirst seen (§7.3.3: a reworded class is a new class)")
	if len(r.FirstSeen) == 0 {
		out.WriteString("\n  none")
	}
	for _, first := range r.FirstSeen {
		out.WriteString("\n  " + w.accent(first.Class) +
			" pr " + strconv.Itoa(first.PR) + " round " + strconv.Itoa(first.Round))
	}
	out.WriteString("\ndemotion candidates, rate over " +
		strconv.FormatFloat(r.Threshold, 'g', -1, 64) + " across " +
		strconv.Itoa(r.MinSamples) + " raise(s) or more")
	if len(r.Demotion) == 0 {
		out.WriteString("\n  none")
	}
	for _, candidate := range r.Demotion {
		out.WriteString("\n  " + w.accent(candidate.Class) + " " +
			strconv.FormatFloat(candidate.RateLowerBound, 'f', 2, 64) +
			" over " + strconv.Itoa(candidate.Raised) + " raise(s)")
	}
	out.WriteString("\n" + r.Bound)
	out.WriteString("\nvolume candidates, `not-here` rate over the same sample")
	if len(r.Volume) == 0 {
		out.WriteString("\n  none")
	}
	for _, candidate := range r.Volume {
		out.WriteString("\n  " + w.accent(candidate.Class) + " " +
			strconv.FormatFloat(candidate.NotHereRate, 'f', 2, 64) +
			" over " + strconv.Itoa(candidate.Raised) + " raise(s)")
	}
	out.WriteString("\n" + finding.VolumeRemedy)
	// §7.3.7, said plainly for the reason `cr rules suggest` says the same
	// thing: a command that prints a list headed "candidates" invites the
	// reading that something has been decided, and the section makes both
	// lists reports and forbids cr to alter a rule's kind by itself.
	out.WriteString("\n§7.3.7: both lists are reports; cr changed no rule")
	return out.String()
}

// statsSample resolves §7.3.4's `stats.demote_threshold` and
// `stats.min_samples` through §2.7's layers.
//
// They are asked of the layers rather than read as constants because they are
// what decides whether a class is proposed for demotion, and a project whose
// reviewers mark false positives diligently needs a different threshold from
// one where the `wrong` marker is rarely used — which §7.3.5 says is the
// ordinary case.
func statsSample(l state.Layout, owner, repo string) (finding.Sample, error) {
	resolved, err := config.Resolve(config.Sources{
		Environ:      os.Environ(),
		GlobalConfig: l.Config(),
		RepoConfig:   l.RepoConfig(owner, repo),
	})
	if err != nil {
		return finding.Sample{}, err
	}
	return finding.Sample{
		Threshold:  resolved.Float("stats.demote_threshold"),
		MinSamples: resolved.Int("stats.min_samples"),
	}, nil
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
				// §7.3.1's vocabulary is closed at five action
				// names and the ledger is a file under ~/.cr a
				// user can open, so a sixth is the ledger cr
				// found and cannot use — the shape
				// state.ContextStoreError already takes, which
				// §11.2 codes 3. Measured 2026-09-12: a
				// hand-edited ledger holding `retracted` exited
				// 2, which told the reader to retype a command
				// line that was right.
				return state.FileFailure(
					"use", layout.RepoTriage(owner, repo), state.UnusableHint, err)
			}
			over, err := statsSample(layout, owner, repo)
			if err != nil {
				return err
			}
			return out.emit(&statsResult{
				Repo: owner + "/" + repo, Events: len(events), TriageReport: report,
				FirstSeen:  finding.FirstSeenClasses(events),
				Threshold:  over.Threshold,
				MinSamples: over.MinSamples,
				Demotion:   finding.DemotionCandidates(report.Classes, over),
				Bound:      finding.DemotionBound,
				Volume:     finding.VolumeCandidates(report.Classes, over),
			})
		},
	}
}
