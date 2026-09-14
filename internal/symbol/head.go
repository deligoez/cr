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

// Unindexed returns the paths, of the round's unit paths given, that index was
// not built over and that declare at least one symbol at rev when read with the
// index's own scanner — deduplicated, in the order given.
//
// It is the per-file counterpart of Head's false. `match.globs` is a
// profile's statement of what its source is, and a changed file outside it is
// source the index holds nothing of: what it adds is never an added symbol,
// what it already declares is never a candidate, and a test calling it
// references nothing the index can name. Left unsaid, §4.3.1's attachment
// reads for that file as "the diff declares nothing here", which is the silent
// skip the item forbids. A file the scanner finds no declaration in loses
// nothing by being outside the index, so it is not returned: an index built
// over it would answer §4.3.1's and §4.4.1's questions exactly as the one
// without it does.
//
// A nil index returns nothing, because Head's false is already the whole-lens
// report, and a path the head does not hold — a file the change deletes — has
// nothing to declare.
func Unindexed(dir, rev string, index *Index, paths []string) ([]string, error) {
	out := make([]string, 0)
	if index == nil {
		return out, nil
	}
	scanner := languages[index.Lang]
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if seen[path] || index.Indexed(path) {
			continue
		}
		seen[path] = true
		lines, held, err := git.FileAtRevision(dir, rev, path)
		if err != nil {
			return nil, err
		}
		if held && len(scanner.scan(File{Path: path, Lines: lines})) > 0 {
			out = append(out, path)
		}
	}
	return out, nil
}
