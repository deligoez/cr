package unit

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/glob"
)

// The kinds §3.4.7 lists without clustering.
const (
	// KindBinary is a file git diffed as binary, which has no hunk to
	// cluster.
	KindBinary = "binary"
	// KindGenerated is a file the repository declares generated: `git
	// check-attr` reports `linguist-generated` set for it at the head.
	KindGenerated = "generated"
	// KindCredential is a file whose name says it carries a secret. It is
	// listed for the reason a binary file is: its content is not something
	// to put in front of a reviewer, and §4.6.1 would put a clustered
	// file's changed lines verbatim into every role's prompt.
	KindCredential = "credential"
)

// credentialNames and credentialExtensions are the built-in fence of §3.4.7's
// KindCredential, matched case-insensitively against a changed file's base
// name. Measured on cr v0.3.1: a tracked `.env` in a pull request's diff was
// matched by no glob (`ignore.globs` defaults to none), was not binary and was
// not generated, so it was clustered and its added lines reached every prompt.
//
// It is not a setting, and that is the fix rather than an omission. §2.7's
// layers replace a list value outright, so a repository that configured any
// related key would silently lose the fence — which is the failure this closes.
// A file listed here is still reported by path, so nothing is hidden from the
// reviewer; what is withheld is its text from a model.
var (
	credentialNames = []string{
		".netrc", ".pgpass", ".htpasswd",
		"credentials", "secrets",
		"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519",
		"id_rsa.pub", "id_dsa.pub", "id_ecdsa.pub", "id_ed25519.pub",
	}
	credentialExtensions = []string{".pem", ".key", ".p12", ".pfx", ".keystore", ".jks"}
)

// credentialShaped reports whether a path's name says it carries a secret.
//
// `.env` and every `.env.<something>` are matched, a template included: a
// reviewer who wants to read `.env.example` opens it, and a fence that tried to
// tell a template from a real file by its name would be guessing about the one
// thing it must not get wrong.
func credentialShaped(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	if name == ".env" || strings.HasPrefix(name, ".env.") {
		return true
	}
	return slices.Contains(credentialNames, name) ||
		slices.Contains(credentialExtensions, filepath.Ext(name))
}

// Listed is one file §3.4.7 lists and does not cluster.
type Listed struct {
	// Path is the file's head-side path.
	Path string `json:"path"`
	// Kind is KindCredential, KindBinary or KindGenerated, tested in that
	// order. A file that is several is listed once, under the first that
	// holds: the fence outranks the rest because a credential file's
	// content must not be clustered whatever else is true of it, and a
	// binary file has no hunk whether or not it is also generated.
	Kind string `json:"kind"`
}

// Files is §3.4.2's and §3.4.7's account of the diff's files that produce no
// unit: how many `ignore.globs` excluded, and which credential-shaped, binary
// and generated files are listed instead of clustered.
type Files struct {
	// Excluded counts the changed files `ignore.globs` matched, per §3.4.2.
	Excluded int `json:"excluded"`
	// Listed are §3.4.7's credential-shaped, binary and generated files,
	// in the order git lists the diff's files. A file `ignore.globs`
	// excluded is counted there and not listed here, unless the fence
	// claimed it first.
	Listed []Listed `json:"listed"`
	// excluded and listed hold the paths behind the two fields, and
	// clustered the changed files that are neither, for Clusterable and
	// the two path accessors.
	excluded  []string
	listed    []string
	clustered []string
}

// FilesOf reads the diff's files from mergeBase to head and sorts them by §3.4.2
// and §3.4.7: listed when the name says it carries a secret, excluded when a
// glob matches the path, listed when git diffed it as binary or head declares
// it generated, and clusterable otherwise.
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
		// The fence is tested before `ignore.globs` so that a
		// credential-shaped file is always named in Listed rather than
		// counted among Excluded: §2.7's layers replace a list value
		// outright, so a repository that sets `ignore.globs` for its
		// own reasons must not be able to move such a file into the
		// silent count, and no ordering of the cases can defeat it.
		case credentialShaped(file.Path):
			files.Listed = append(files.Listed, Listed{Path: file.Path, Kind: KindCredential})
			files.listed = append(files.listed, file.Path)
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
