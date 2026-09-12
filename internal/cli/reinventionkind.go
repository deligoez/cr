package cli

import (
	"os"
	"slices"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/reinvention"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/symbol"
)

// defaultReinventionQuestions is §4.3.4: a reinvention item defaults to
// `kind: question`, because cr cannot know whether the existing symbol was
// looked at and rejected for a reason.
//
// A record is a reinvention item when it does what §4.3.3 has one do: it cites
// the candidate symbol as `path:line`. The candidates are §4.3.1's, attached to
// a symbol added inside the record's own unit and computed the way `cr review`
// computed the ones its prompt showed — the same head index, the same round
// diff, the same ranking. The match is positional, as §6.2.5's origin stamp is,
// so cr judges nothing about what the record says; and a citation that happens
// to name a candidate's declaration line is the reinvention question whatever
// else it argues, which is why no axis narrows it.
//
// It runs before §6.3's forcing and independently of it. A record graded
// `cited` is not forced, and the default still holds for it: §4.3.4 is about
// what cr cannot know, which a citation does not supply. The move only ever
// lowers the register, so a false match costs a question and never an
// assertion.
//
// The repository and the pull request are read only when some record cites
// anything and the round's profile declares a language cr indexes; with no
// index §4.3.1 attached no candidate, so no citation can name one.
func defaultReinventionQuestions(
	l state.Layout, owner, repo string, pr int, round *state.Meta, formed []roundUnit, records []*finding.Finding,
) error {
	if !slices.ContainsFunc(records, func(record *finding.Finding) bool { return len(record.Citations) > 0 }) {
		return nil
	}
	attached, err := roundCandidates(l, owner, repo, pr, round)
	if err != nil {
		return err
	}
	for _, record := range records {
		if citesACandidate(record, ownUnit(formed, record.Unit), attached) {
			record.Kind = finding.KindQuestion
		}
	}
	return nil
}

// roundCandidates is §4.3.1's attachment for the round, or none when the head
// has no index.
func roundCandidates(
	l state.Layout, owner, repo string, pr int, round *state.Meta,
) ([]reinvention.Attachment, error) {
	p, err := statusProfile(l, round.ProfileID)
	if err != nil {
		return nil, err
	}
	dir, err := repoDir()
	if err != nil {
		return nil, err
	}
	index, built, err := symbol.Head(dir, round.Head, p)
	if err != nil || !built {
		return nil, err
	}
	resolved, err := config.Resolve(config.Sources{
		Environ:      os.Environ(),
		GlobalConfig: l.Config(),
		RepoConfig:   l.RepoConfig(owner, repo),
	})
	if err != nil {
		return nil, err
	}
	hunks, err := roundHunks(owner, repo, pr, round.Head)
	if err != nil {
		return nil, err
	}
	return reinvention.Attach(p, index, hunks, reinvention.Ranking{
		MinSimilarity: resolved.Float("reinvention.min_similarity"),
		MaxCandidates: resolved.Int("reinvention.max_candidates"),
	}).Attached, nil
}

// ownUnit is the round's unit by id, and nil when the round holds none.
func ownUnit(formed []roundUnit, id string) *roundUnit {
	for i := range formed {
		if formed[i].ID == id {
			return &formed[i]
		}
	}
	return nil
}

// citesACandidate reports whether one of the record's citations names, by path
// and line, a candidate attached to a symbol added inside its own unit. Only a
// RIGHT unit can hold an added symbol, for the reason review's addedIn gives:
// the index is built over the head.
func citesACandidate(record *finding.Finding, own *roundUnit, attached []reinvention.Attachment) bool {
	if own == nil || own.Side != git.Right {
		return false
	}
	for i := range attached {
		if !own.Contains(attached[i].Added.Path, attached[i].Added.Line) {
			continue
		}
		for _, candidate := range attached[i].Candidates {
			if slices.ContainsFunc(record.Citations, func(cited finding.Citation) bool {
				return cited.Path == candidate.Path && cited.Line == candidate.Line
			}) {
				return true
			}
		}
	}
	return false
}
