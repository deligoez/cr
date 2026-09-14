package review

import (
	"time"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/reinvention"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/symbol"
	"github.com/deligoez/cr/internal/testadequacy"
	"github.com/deligoez/cr/internal/unit"
)

// attach fills the attachments computed over the round's diff: §4.3.1's
// candidates, §4.3.6's rule hits with §2.6.1.4's injected standards, and
// §4.4.1's test files. It returns §4.5.4's report of the halves that could not
// run, which travels with the prompts rather than in a report a caller has to
// remember to ask for.
func (r *Round) attach(
	src *Sources, p *profile.Profile, hunks []git.Hunk,
) ([]finding.HonestyDisclosure, error) {
	// A head §4.3.1 cannot index answers false with a nil index, which
	// reinvention.Attach turns into its unavailability rather than into an
	// empty list of candidates.
	index, _, err := symbol.Head(src.RepoDir, r.Head, p)
	if err != nil {
		return nil, err
	}
	r.Candidates = reinvention.Attach(p, index, hunks, reinvention.Ranking{
		MinSimilarity: src.Config.Float("reinvention.min_similarity"),
		MaxCandidates: src.Config.Int("reinvention.max_candidates"),
	})
	// A unit whose file declares symbols the index was not built over is
	// one neither half can speak for, and both say so, naming the files.
	paths := r.unitPaths()
	unindexed, err := symbol.Unindexed(src.RepoDir, r.Head, index, paths)
	if err != nil {
		return nil, err
	}
	r.Candidates.MarkUnindexed(p, unindexed)
	if err := r.detect(src, p, hunks); err != nil {
		return nil, err
	}
	// The symbol half of §4.4.1 reads the round's test files at the head
	// against the same index. With no index, refs is nil and that half
	// reports itself unavailable with its reason, per testadequacy.Attach,
	// while the file half still runs.
	refs, err := testadequacy.HeadReferences(src.RepoDir, r.Head, p, index, hunks)
	if err != nil {
		return nil, err
	}
	tests := testadequacy.Attach(p, refs, hunks)
	tests.Unavailable = append(tests.Unavailable, testadequacy.Unindexed(p, paths, unindexed)...)
	r.Tests = testadequacy.PerUnit(r.clusters(), tests)

	// The halves are handed back as disclosures rather than as finished
	// sentences, because §4.5.4's report has four kinds and this function
	// sees two of them. Run collects all four through coverage.Lenses,
	// which is the only place §4.6.4's roles are known; flattening here
	// would produce a list that has to be re-joined there anyway, and the
	// joining is what the collector exists for.
	halves := make([]finding.HonestyDisclosure, 0,
		len(r.Candidates.Unavailable)+len(tests.Unavailable))
	for _, out := range r.Candidates.Unavailable {
		halves = append(halves, out)
	}
	for _, out := range tests.Unavailable {
		halves = append(halves, out)
	}
	return halves, nil
}

// detect resolves §2.6's corpus for the round's profile, runs §2.6.1's
// detectors over the diff, and places each hit on the unit that contains it.
//
// A rule whose `profiles` names other profiles is left out of the round, by
// the same rule.ForProfile `cr rules check` asks.
func (r *Round) detect(src *Sources, p *profile.Profile, hunks []git.Hunk) error {
	path := ""
	if p.ID != "" {
		path = src.Layout.Profile(p.ID)
	}
	corpus, err := rule.Resolve(
		src.Layout.RepoRulesDir(src.Owner, src.Repo), src.Layout.RulesDir(), path, p.Rules)
	if err != nil {
		return err
	}
	r.Rules = rule.ForProfile(corpus, p.ID)
	matchers, err := rule.Compile(r.Rules)
	if err != nil {
		return err
	}
	r.Matchers = matchers
	formed := make([]unit.Unit, 0, len(r.Units))
	for i := range r.Units {
		formed = append(formed, r.Units[i].Unit)
	}
	hits := rule.Evaluate(matchers, hunks)
	// §2.6.1.6: every hit reaches the repository's ledger through the
	// writer `cr rules check` uses, keyed so a second run at the same head
	// overwrites rather than counts it again. §6.2.5 stamps a citation
	// `origin: rule` only against this file, so a hit attached to a prompt
	// and left out of it would grade the agent's confirmation as its own.
	if err := rule.RecordHits(src.Layout, src.Owner, src.Repo, hits, &rule.Occasion{
		PR: src.PR, Round: r.Round, Head: r.Head, At: time.Now(),
	}); err != nil {
		return err
	}
	r.Hits = rule.Attach(formed, hits)
	return nil
}

// unitPaths are the files of the round's units, in Units' order.
func (r *Round) unitPaths() []string {
	paths := make([]string, 0, len(r.Units))
	for i := range r.Units {
		paths = append(paths, r.Units[i].Path)
	}
	return paths
}

// clusters are the round's units in the shape §3.4.4 formed them, which is the
// shape testadequacy.PerUnit hands §4.4.1's attachment to.
func (r *Round) clusters() []unit.Cluster {
	clusters := make([]unit.Cluster, 0, len(r.Units))
	for i := range r.Units {
		u := &r.Units[i]
		clusters = append(clusters, unit.Cluster{
			Path: u.Path, Side: u.Side, Formation: u.Formation, Hunks: u.Hunks, Oversized: u.Oversized,
		})
	}
	return clusters
}
