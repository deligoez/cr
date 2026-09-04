package symbol

import (
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/glob"
	"github.com/deligoez/cr/internal/profile"
)

// Head builds §4.3.1's symbol index over the head of the repository at dir.
//
// It reports false when the profile declares no `symbols.lang`, or names one cr
// has no scanner for. That is the branch §4.3.1 sends to §4.5.4, and it is
// answered before a single git read: a head cr cannot index costs nothing to
// not index.
//
// # What it reads
//
// The files the profile owns, which is `match.globs`. A profile's globs are its
// own statement of what its source is, and an index over everything else — a
// vendored dependency tree, a build artefact, a fixture — would fill §4.3.1's
// candidate pool with symbols no author of this pull request wrote, so every
// added symbol would be answered with a question about code nobody here
// maintains. A profile declaring no globs owns no source and indexes nothing,
// which is the same statement read the other way.
//
// It reads the revision and never the worktree, for the reason
// git.BlobsAtRevision gives, and it writes nothing: invariant 2 leaves the
// repository under review read-only, and the index lives in memory here and
// under `~/.cr/` wherever a caller persists it.
func Head(dir, rev string, p *profile.Profile) (*Index, bool, error) {
	if !Supported(p.Symbols.Lang) {
		return nil, false, nil
	}
	blobs, err := git.BlobsAtRevision(dir, rev)
	if err != nil {
		return nil, false, err
	}
	files := make([]File, 0, len(blobs))
	for _, blob := range blobs {
		if !glob.MatchAny(p.Match.Globs, blob.Path) {
			continue
		}
		lines, err := git.BlobLines(dir, blob.Object)
		if err != nil {
			return nil, false, err
		}
		files = append(files, File{Path: blob.Path, Lines: lines})
	}
	index, built := Build(p.Symbols.Lang, files)
	return index, built, nil
}
