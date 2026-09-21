package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
)

// rulesSuggestResult is §2.6.3.2's report: the candidates, and the threshold
// they were held to.
//
// The threshold is reported because it is configurable and because nothing else
// in the document explains an empty answer. A run that found nothing and a run
// whose `rules.harvest_min` was raised to twelve print the same candidates, and
// only this number tells them apart.
//
// There is no field here through which a rule could be written. §2.6.3.3 makes
// candidates a report and nothing more, so the command's whole output is this
// document and its whole effect is printing it.
type rulesSuggestResult struct {
	// Min is the `rules.harvest_min` the run resolved.
	Min int `json:"harvest_min"`
	// Scanned is how many posted comments the scan read, which is the
	// denominator of everything below.
	Scanned int `json:"scanned"`
	// Candidates are the groups that reached Min, in the order they were
	// first seen.
	Candidates []rule.Candidate `json:"candidates"`
}

// Text names the threshold and what met it, then each candidate by class and
// occurrence count, and says plainly that cr wrote nothing.
//
// The last part is not decoration. A command called `suggest` that prints a
// rule-shaped thing invites the reading that the rule now exists, and §2.6.3.3
// forbids exactly that — so the line that would be a reassurance elsewhere is
// the section's requirement here.
func (r *rulesSuggestResult) Text(w *writer) string {
	var out strings.Builder
	out.WriteString(w.accent(strconv.Itoa(len(r.Candidates))) +
		" candidate rule(s) from " + strconv.Itoa(r.Scanned) +
		" posted comment(s), at " + strconv.Itoa(r.Min) + " occurrence(s) or more")
	for _, candidate := range r.Candidates {
		out.WriteString("\n  " + candidate.Class + ": " +
			strconv.Itoa(candidate.Occurrences) + " occurrence(s)")
	}
	out.WriteString("\nCandidates are reported only (§2.6.3.3); cr wrote no rule file")
	return out.String()
}

// newRulesSuggestCmd runs §11's `cr rules suggest --repo <owner/repo>`,
// §2.6.3.1's scan of the bodies of comments posted from recorded rounds,
// grouped by class and by normalised body per §1.4.
//
// The scope is the repository and not a pull request, which is why the command
// takes no `<pr>`: a rule candidate is a thing a reviewer keeps saying on this
// project, and a scan of one pull request could not see the repetition that
// makes it one.
//
// §2.6.3.3 makes it a report and nothing more: cr must not write a rule file by
// itself, so there is no flag here that would have it do so, and nothing below
// this line opens a file for writing.
func newRulesSuggestCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "suggest",
		Short: "Propose rules from recurring comment history",
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
			threshold, err := harvestMin(layout, owner, repo)
			if err != nil {
				return err
			}
			posted, err := postedComments(layout, owner, repo)
			if err != nil {
				return err
			}
			candidates, err := rule.Harvest(posted, threshold)
			if err != nil {
				return err
			}
			return out.emit(&rulesSuggestResult{
				Min: threshold, Scanned: len(posted), Candidates: candidates,
			})
		},
	}
}

// harvestMin resolves §2.6.3.2's `rules.harvest_min`, default 3.
//
// It is asked of the §2.4 layers rather than read as a constant, because the
// threshold is what decides whether a repetition is a pattern and a project
// that comments a great deal will want a different number from one that
// comments rarely.
func harvestMin(l state.Layout, owner, repo string) (int, error) {
	resolved, err := config.Resolve(config.Sources{
		Environ:      os.Environ(),
		GlobalConfig: l.Config(),
		RepoConfig:   l.RepoConfig(owner, repo),
	})
	if err != nil {
		return 0, err
	}
	return resolved.Int("rules.harvest_min"), nil
}

// postedComments is §2.6.3.1's scan: every record of the repository that
// reached `posted`, with the body its round's draft carries for it. A record
// `cr verify` or `cr withdraw` has since moved on was posted all the same, so
// the test is State.Sent rather than the state `posted` alone.
//
// The body is read out of `rounds/<n>/draft.md` rather than off the record,
// because §8.1.2 makes editing that file the one input path for reader-facing
// prose: the record holds the English summary and evidence a role wrote, and
// what the author received is what the reviewer left in the draft. Grouping on
// the summary would group what cr proposed rather than what was said.
//
// A record whose draft holds no block for it is skipped rather than refused.
// §9.3.5 keeps earlier rounds and their drafts, but a draft is a file on disk
// that a user may have removed, and a repository's whole harvest is not worth
// refusing over one missing round.
//
// Every read here is lock-free per §2.3.2, and every one of them is a read.
func postedComments(l state.Layout, owner, repo string) ([]rule.Comment, error) {
	prs, err := recordedPRs(l, owner, repo)
	if err != nil {
		return nil, err
	}
	comments := make([]rule.Comment, 0)
	for _, pr := range prs {
		stored, err := state.ReadRecords[finding.Finding](l, owner, repo, pr, state.FileFindings)
		if err != nil {
			return nil, err
		}
		drafts := make(map[int]map[string]string)
		for i := range stored {
			if !stored[i].State.Sent() {
				continue
			}
			bodies, err := roundBodies(l, owner, repo, pr, stored[i].Round, drafts)
			if err != nil {
				return nil, err
			}
			body, drafted := bodies[stored[i].ID]
			if !drafted {
				continue
			}
			comments = append(comments, rule.Comment{
				PR: pr, Round: stored[i].Round, Record: stored[i].ID,
				Class: stored[i].Class, Body: body,
			})
		}
	}
	return comments, nil
}

// roundBodies is one round's draft read into record id and body, remembered
// across the records of the same round so a draft is parsed once however many
// comments it carries.
//
// A round with no draft file is remembered as holding no block, so a missing
// file is read from disk once rather than on every record of that round.
func roundBodies(
	l state.Layout, owner, repo string, pr, round int, drafts map[int]map[string]string,
) (map[string]string, error) {
	if known, read := drafts[round]; read {
		return known, nil
	}
	body, err := l.ReadRound(owner, repo, pr, round, state.FileDraft)
	if errors.Is(err, fs.ErrNotExist) {
		drafts[round] = map[string]string{}
		return drafts[round], nil
	}
	if err != nil {
		return nil, err
	}
	bodies, err := draft.Bodies(string(body))
	if err != nil {
		return nil, err
	}
	drafts[round] = bodies
	return bodies, nil
}

// recordedPRs is the pull requests of one repository that have state, in
// ascending order.
//
// The order is imposed rather than taken from the filesystem: §2.1.1 requires
// the same inputs to give the same result, and directory order is neither
// sorted nor stable across systems — `pr-2` and `pr-10` sort the wrong way as
// text, so the numbers are parsed and compared as numbers.
//
// A repository no round has been opened on has no state directory at all, which
// is an empty scan rather than a refusal: a project that has never been
// reviewed has nothing to harvest, and that is an answer.
func recordedPRs(l state.Layout, owner, repo string) ([]int, error) {
	entries, err := os.ReadDir(l.RepoStateDir(owner, repo))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	prs := make([]int, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		number, numbered := strings.CutPrefix(filepath.Base(entry.Name()), "pr-")
		pr, err := strconv.Atoi(number)
		if !numbered || err != nil {
			continue
		}
		prs = append(prs, pr)
	}
	slices.Sort(prs)
	return prs, nil
}
