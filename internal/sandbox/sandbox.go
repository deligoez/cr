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

	"github.com/deligoez/cr/internal/git"
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
	// worktree is registered in.
	RepoDir string
}

// Result is what one creation did.
type Result struct {
	// Path is the sandbox worktree.
	Path string
	// Head is the revision it was checked out at.
	Head string
}

// Create makes the sandbox worktree of §5.1.1.
func Create(src *Sources) (*Result, error) {
	path := src.Layout.Sandbox(src.Owner, src.Repo, src.PR)
	switch _, err := os.Stat(path); {
	case err == nil:
		return nil, &ExistsError{Owner: src.Owner, Repo: src.Repo, PR: src.PR, Path: path}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("cannot inspect %s: %w", path, err)
	}
	if err := git.AddWorktree(src.RepoDir, path, src.Head); err != nil {
		return nil, err
	}
	return &Result{Path: path, Head: src.Head}, nil
}
