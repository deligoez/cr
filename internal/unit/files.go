package unit

import (
	"slices"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/glob"
)

// The two kinds §3.4.7 lists without clustering.
const (
	// KindBinary is a file git diffed as binary, which has no hunk to
	// cluster.
	KindBinary = "binary"
	// KindGenerated is a file the repository declares generated: `git
	// check-attr` reports `linguist-generated` set for it at the head.
	KindGenerated = "generated"
)

// Listed is one file §3.4.7 lists and does not cluster.
type Listed struct {
	// Path is the file's head-side path.
	Path string `json:"path"`
	// Kind is KindBinary or KindGenerated. A file that is both is listed
	// once, as binary, because a binary file has no hunk whether or not it
	// is also generated.
	Kind string `json:"kind"`
}

// Files is §3.4.2's and §3.4.7's account of the diff's files that produce no
// unit: how many `ignore.globs` excluded, and which binary and generated files
// are listed instead of clustered.
type Files struct {
	// Excluded counts the changed files `ignore.globs` matched, per §3.4.2.
	Excluded int `json:"excluded"`
	// Listed are §3.4.7's binary and generated files, in the order git
	// lists the diff's files. A file `ignore.globs` excluded is counted
	// there and not listed here.
	Listed []Listed `json:"listed"`
	// excluded and listed hold the paths behind the two fields, and
	// clustered the changed files that are neither, for Clusterable and
	// the two path accessors.
	excluded  []string
	listed    []string
	clustered []string
}

// FilesOf reads the diff's files from mergeBase to head and sorts them by §3.4.2
// and §3.4.7: excluded when a glob matches the path, listed when git diffed it
// as binary or head declares it generated, and clusterable otherwise.
//
// The generated question is asked only of the files no glob excluded, since an
// excluded file is not listed whatever its attributes say.
func FilesOf(dir, mergeBase, head string, globs []string) (Files, error) {
	changed, err := git.ChangedFiles(dir, mergeBase, head)
	if err != nil {
		return Files{}, err
	}
	kept := make([]string, 0, len(changed))
	for i := range changed {
		if !glob.MatchAny(globs, changed[i].Path) {
			kept = append(kept, changed[i].Path)
		}
	}
	generated, err := git.Generated(dir, head, kept)
	if err != nil {
		return Files{}, err
	}
	return sortFiles(changed, globs, generated), nil
}

// sortFiles is FilesOf's decision over files already read.
func sortFiles(changed []git.ChangedFile, globs []string, generated map[string]bool) Files {
	files := Files{Listed: make([]Listed, 0)}
	for i := range changed {
		file := &changed[i]
		switch {
		case glob.MatchAny(globs, file.Path):
			files.Excluded++
			files.excluded = append(files.excluded, file.Path)
		case file.Binary:
			files.Listed = append(files.Listed, Listed{Path: file.Path, Kind: KindBinary})
			files.listed = append(files.listed, file.Path)
		case generated[file.Path]:
			files.Listed = append(files.Listed, Listed{Path: file.Path, Kind: KindGenerated})
			files.listed = append(files.listed, file.Path)
		case file.Changed:
			files.clustered = append(files.clustered, file.Path)
		}
	}
	return files
}

// Clusterable keeps the hunks of the files neither excluded nor listed, which
// are the only hunks §3.4.4 clusters.
func (f *Files) Clusterable(hunks []git.Hunk) []git.Hunk {
	kept := make([]git.Hunk, 0, len(hunks))
	for i := range hunks {
		if !slices.Contains(f.excluded, hunks[i].Path) && !slices.Contains(f.listed, hunks[i].Path) {
			kept = append(kept, hunks[i])
		}
	}
	return kept
}

// ExcludedPaths returns the paths `ignore.globs` excluded, in the order git
// lists them.
func (f *Files) ExcludedPaths() []string {
	return slices.Clone(f.excluded)
}

// ClusteredPaths returns the paths whose hunks Clusterable keeps: the files
// with a changed line that no glob excluded and §3.4.7 did not list. Every one
// of them produces at least one unit.
func (f *Files) ClusteredPaths() []string {
	return slices.Clone(f.clustered)
}
