// Package sandbox creates and prepares the probe worktree of §5.1.
//
// It is the first code in cr that writes into a checkout and runs commands in
// one, and the halves are kept in the packages that already own them: the
// worktree is added through internal/git, which is cr's only door to git, and
// every path written is derived through internal/state, which §2.2 roots at
// ~/.cr. What lives here is the order the two happen in.
package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// ExistsError reports a sandbox that is already on disk.
//
// §5.1.1 has `cr sandbox create` create the worktree, and §5.1.5 gives removing
// one to `cr sandbox destroy`. Creating over an existing sandbox would mean
// deleting a checkout cr did not just make — one that may be mid-run, or
// carrying the mutation §5.3.3 is about to revert — so the situation is
// reported instead. §11.2 codes it 4: the command line is right and every file
// named was read, and what refuses is where the pull request stands.
//
// §5.1.6's recreation is a different act with a different trigger. It follows a
// failed cleanliness check, which is that check's own to make.
type ExistsError struct {
	// Owner, Repo, and PR name the pull request whose sandbox is there.
	Owner string
	Repo  string
	PR    int
	// Path is the sandbox directory that already exists.
	Path string
}

func (e *ExistsError) Error() string {
	return fmt.Sprintf(
		"%s already exists: §5.1.1 creates the sandbox worktree and §5.1.5 removes it, "+
			"and cr does not delete a checkout it did not just make; "+
			"run `cr sandbox destroy %d --repo %s/%s` first",
		e.Path, e.PR, e.Owner, e.Repo,
	)
}

// Sources is everything creating a sandbox needs, and nothing it could decide
// for itself.
//
// The head in particular is the round's recorded one rather than anything this
// package resolves: §9.3.1 makes meta.json's head the round's identity, and a
// sandbox checked out at a head the round never saw would be probing code no
// finding was written against.
type Sources struct {
	// Layout derives the sandbox path of §5.1.1.
	Layout state.Layout
	// Owner, Repo, and PR name the pull request.
	Owner string
	Repo  string
	PR    int
	// Head is the pull request head the worktree is checked out at.
	Head string
	// RepoDir is the repository under review, which is the checkout the
	// worktree is registered in, and the one §5.1.2 copies from.
	RepoDir string
	// Copy is the resolved profile's `sandbox.copy` (§2.4, §5.1.2).
	Copy []string
	// Setup is the resolved profile's `sandbox.setup` (§2.4, §5.1.3).
	Setup []string
	// ProfileFile is the file those two came out of. It carries no
	// behaviour and exists so a refusal can name the file the user has to
	// open, which is what §2.5 item 3 requires of the abort.
	ProfileFile string
}

// Result is what one creation did.
type Result struct {
	// Path is the sandbox worktree.
	Path string
	// Head is the revision it was checked out at.
	Head string
	// Copied are the `sandbox.copy` paths that were copied in, in profile
	// order.
	Copied []string
	// Absent are the `sandbox.copy` paths the checkout did not hold. They
	// are reported rather than dropped: a suite that fails for want of a
	// `vendor` tree nobody copied is a failure whose cause is otherwise
	// invisible, and §5.1.3's setup is what is expected to supply them.
	Absent []string
	// Setup are the `sandbox.setup` commands that ran, in the order they
	// ran in.
	Setup []string
}

// Create makes the sandbox worktree of §5.1.1, fills it with §5.1.2's copied
// paths, and then runs §5.1.3's setup commands in it.
//
// The order is the contract, not an implementation detail. §5.1.2 copies "after
// creation" and §5.1.3 runs "once, in order, in the sandbox root", and a setup
// command exists to act on what the copies brought: `composer install` reads a
// copied `.env` and completes a copied `vendor`. So every copy finishes before
// the first command starts, and the commands then run one after another in the
// order the profile lists them.
//
// The steps are validated before the worktree is added. A profile that cannot
// be carried out should cost nothing, and a run that failed halfway would leave
// a sandbox behind that §5.1.1 then refuses to create over.
func Create(src *Sources) (*Result, error) {
	path := src.Layout.Sandbox(src.Owner, src.Repo, src.PR)
	switch _, err := os.Stat(path); {
	case err == nil:
		return nil, &ExistsError{Owner: src.Owner, Repo: src.Repo, PR: src.PR, Path: path}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("cannot inspect %s: %w", path, err)
	}
	if err := src.steps(); err != nil {
		return nil, err
	}
	if err := git.AddWorktree(src.RepoDir, path, src.Head); err != nil {
		return nil, err
	}

	created := &Result{
		Path:   path,
		Head:   src.Head,
		Copied: make([]string, 0, len(src.Copy)),
		Absent: make([]string, 0, len(src.Copy)),
		Setup:  make([]string, 0, len(src.Setup)),
	}
	for _, rel := range src.Copy {
		copied, err := src.Layout.CopyIntoSandbox(src.Owner, src.Repo, src.PR, src.RepoDir, rel)
		if err != nil {
			return nil, err
		}
		if copied {
			created.Copied = append(created.Copied, rel)
			continue
		}
		created.Absent = append(created.Absent, rel)
	}
	for _, command := range src.Setup {
		if err := runSetup(setupArgv(command), path); err != nil {
			return nil, err
		}
		created.Setup = append(created.Setup, command)
	}
	return created, nil
}

// steps holds §5.1.2's paths and §5.1.3's commands to what they have to be for
// the sandbox to survive them.
//
// A profile is data the user writes, and two of its entries would do something
// other than what the section describes: a copy path that leaves the checkout
// writes outside the sandbox, which invariant 2 does not permit anywhere, and
// an entry with no command in it names no program to run. §2.5 item 3 gives a
// profile field cr cannot use exit code 3 with the file and the field named,
// which is what MalformedError carries.
func (src *Sources) steps() error {
	for _, rel := range src.Copy {
		if err := copyPath(src.ProfileFile, rel); err != nil {
			return err
		}
	}
	for _, command := range src.Setup {
		if len(setupArgv(command)) == 0 {
			return &profile.MalformedError{
				File:    src.ProfileFile,
				Field:   "sandbox.setup",
				Problem: "holds an entry with no command in it",
			}
		}
	}
	return nil
}

// copyPath holds one `sandbox.copy` entry to a path inside the checkout.
func copyPath(file, rel string) error {
	cleaned := filepath.Clean(rel)
	switch {
	case rel == "" || cleaned == ".":
		return &profile.MalformedError{
			File:    file,
			Field:   "sandbox.copy",
			Problem: "holds an empty entry, which addresses the whole checkout rather than a path in it",
		}
	case filepath.IsAbs(rel):
		return &profile.MalformedError{
			File:  file,
			Field: "sandbox.copy",
			Problem: fmt.Sprintf(
				"holds the absolute path %q; §5.1.2 copies paths from the main checkout, which are relative to it", rel),
		}
	case cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)):
		return &profile.MalformedError{
			File:  file,
			Field: "sandbox.copy",
			Problem: fmt.Sprintf(
				"holds %q, which leaves the checkout and would write outside the sandbox", rel),
		}
	}
	return nil
}
