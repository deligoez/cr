package cli

import (
	"os"
	"slices"
	"strconv"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// statusFiles re-derives §3.4.2's excluded count and §3.4.7's listing for the
// round, and reports every path on which `ignore.globs` as resolved now
// disagrees with the units the round was formed with.
//
// Nothing in §2.3 holds either value, so they are derived again rather than
// read, from the diff at the round's recorded head — the precedent lensesOf
// sets for §4.3.1's halves, which reads the repository and the pull request on
// this path already. The binary and generated answers are facts about that
// head and come out the same on every run. The globs are not: they are read
// from the configuration as it now stands, and the round's units are the only
// record of what they said when it was formed. So a disagreement is disclosed
// rather than re-counted silently: a path the globs now exclude that holds a
// unit, and a path they no longer exclude that holds none.
func statusFiles(
	l state.Layout, owner, repo string, pr int, round *state.Meta,
) (unit.Files, []string, error) {
	resolved, err := config.Resolve(config.Sources{
		Environ:      os.Environ(),
		GlobalConfig: l.Config(),
		RepoConfig:   l.RepoConfig(owner, repo),
	})
	if err != nil {
		return unit.Files{}, nil, err
	}
	dir, err := repoDir()
	if err != nil {
		return unit.Files{}, nil, err
	}
	opened, err := ghClient().PullRequest(owner, repo, pr)
	if err != nil {
		return unit.Files{}, nil, err
	}
	mergeBase, err := git.MergeBase(dir, opened.Base, round.Head)
	if err != nil {
		return unit.Files{}, nil, err
	}
	files, err := unit.FilesOf(dir, mergeBase, round.Head, resolved.Strings("ignore.globs"))
	if err != nil {
		return unit.Files{}, nil, err
	}
	formed, err := roundUnitsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return unit.Files{}, nil, err
	}
	drift := globDrift(&files, formed, round.Round)
	return files, drift, nil
}

// globDrift words the two disagreements statusFiles discloses, excluded paths
// first, each in the order git lists the diff's files.
func globDrift(files *unit.Files, formed []roundUnit, round int) []string {
	holding := make([]string, 0, len(formed))
	for i := range formed {
		holding = append(holding, formed[i].Path)
	}
	said := make([]string, 0)
	for _, path := range files.ExcludedPaths() {
		if slices.Contains(holding, path) {
			said = append(said, "§3.4.2: "+path+" holds a unit of round "+strconv.Itoa(round)+
				", but ignore.globs as resolved now excludes it, so the excluded count is not the one the round was formed under")
		}
	}
	for _, path := range files.ClusteredPaths() {
		if !slices.Contains(holding, path) {
			said = append(said, "§3.4.2: "+path+" holds no unit of round "+strconv.Itoa(round)+
				", but ignore.globs as resolved now does not exclude it, so the excluded count is not the one the round was formed under")
		}
	}
	return said
}
