package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/deligoez/cr/internal/git"
)

// EnvFiles sets the clone root's gitignored `.env*` files beside the sandbox.
//
// A worktree is added at a revision, so it holds what the commit holds and
// nothing the clone keeps out of version control. An ignored `.env.testing` is
// exactly such a file, and a Laravel suite started without it reads `.env`
// instead — in the field trial spec/field-feedback.md item 2.1 records, the
// developer's own application database. cr forms no judgement about which of
// these files matters: it lists them, says which the sandbox holds, and names
// the ones nothing copied, so the difference is said at the moment of the run.
type EnvFiles struct {
	// Root is the clone root the files were listed at.
	Root string
	// Ignored are the clone root's gitignored `.env*` files.
	Ignored []string
	// Held are those of Ignored the sandbox root holds.
	Held []string
	// Uncopied are those of Ignored the sandbox root does not hold and
	// `sandbox.copy` does not name.
	Uncopied []string
	// ProfileFile is the profile whose `sandbox.copy` would copy them.
	ProfileFile string
}

// CompareEnvFiles lists src's clone root's gitignored `.env*` files through git
// and asks the sandbox at path which of them it holds.
//
// Nothing is opened: the clone's files are names git lists, and the sandbox's
// are an Lstat each, so a symbolic link a `sandbox.copy` reproduced counts as
// held whether or not it resolves.
func CompareEnvFiles(src *Sources, path string) (*EnvFiles, error) {
	root, err := git.Toplevel(src.RepoDir)
	if err != nil {
		return nil, err
	}
	ignored, err := git.IgnoredEnvFiles(root)
	if err != nil {
		return nil, err
	}
	compared := &EnvFiles{
		Root: root, Ignored: ignored, ProfileFile: src.ProfileFile,
		Held: make([]string, 0, len(ignored)), Uncopied: make([]string, 0, len(ignored)),
	}
	for _, name := range ignored {
		switch _, err := os.Lstat(filepath.Join(path, name)); {
		case err == nil:
			compared.Held = append(compared.Held, name)
		case !errors.Is(err, fs.ErrNotExist):
			return nil, fmt.Errorf("cannot inspect %s: %w", filepath.Join(path, name), err)
		case !slices.ContainsFunc(src.Copy, func(entry string) bool { return filepath.Clean(entry) == name }):
			compared.Uncopied = append(compared.Uncopied, name)
		}
	}
	return compared, nil
}

// Disclosures names every uncopied file, one sentence each, with the field
// that copies it.
//
// A round no profile matched has no file to name, and the sentence then names
// the field alone.
func (e *EnvFiles) Disclosures() []string {
	field := "sandbox.copy"
	if e.ProfileFile != "" {
		field += " in " + e.ProfileFile
	}
	sentences := make([]string, 0, len(e.Uncopied))
	for _, name := range e.Uncopied {
		sentences = append(sentences, fmt.Sprintf(
			"%s is gitignored at the clone root %s and absent from the sandbox, so the suite runs without it; "+
				"add it to %s to copy it in", name, e.Root, field))
	}
	return sentences
}
