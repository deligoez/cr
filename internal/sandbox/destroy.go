package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/deligoez/cr/internal/git"
)

// Removed is what one destruction did.
type Removed struct {
	// Path is the sandbox that is no longer there.
	Path string
	// Existed says whether there was a worktree at that path to remove.
	// A pull request with no sandbox is not a failure — §5.1.5 asks for
	// the worktree and its registration to be gone, and they are — but it
	// is a different thing to tell the reader, who may have destroyed it
	// already or never created it.
	Existed bool
}

// Destroy removes the sandbox worktree and its registration (§5.1.5).
//
// The registration is the half that has to be said out loud. `git worktree
// remove` deletes both, but a directory that is already gone leaves the
// registration behind under the repository's `.git/worktrees/`, and
// `worktree add` then refuses the path as already registered — so the one
// state §5.1.1 could not recover from is exactly the one a destruction that
// only deleted a directory would leave. removeSandbox prunes in both branches.
//
// The post-setup baseline goes with it. §5.1.6 makes the baseline a
// description of one sandbox's tracked-file state, and the sandbox it
// describes no longer exists: Create invalidates it before the worktree is
// added for the same reason pointed the other way, and leaving a record behind
// that answers for a checkout that is gone is the thing that reasoning was
// about.
func Destroy(src *Sources) (*Removed, error) {
	path := src.Layout.Sandbox(src.Owner, src.Repo, src.PR)
	existed, err := removeSandbox(src, path)
	if err != nil {
		return nil, err
	}
	if err := src.invalidateBaseline(); err != nil {
		return nil, err
	}
	return &Removed{Path: path, Existed: existed}, nil
}

// removeSandbox deletes the worktree at path and every registration naming a
// directory that is not there, and reports whether there was a worktree to
// delete.
//
// It is shared with §5.1.6's recreation, which needs the same branches for the
// same reason and differs only in what it does afterwards.
func removeSandbox(src *Sources, path string) (bool, error) {
	switch _, err := os.Stat(path); {
	case err == nil:
		return true, removeDirectory(src, path)
	case errors.Is(err, fs.ErrNotExist):
		// No directory to remove, but possibly a registration naming
		// one, which `worktree add` would refuse the path over.
		return false, git.PruneWorktrees(src.RepoDir)
	default:
		return false, fmt.Errorf("cannot inspect %s: %w", path, err)
	}
}

// removeDirectory removes a sandbox directory that is there, through git when
// the repository still lists it as a worktree and as cr's own files when it
// does not.
//
// The second branch is the one a re-cloned repository under review reaches: the
// directory survives, its registration does not, and `worktree remove` refuses
// it as "not a working tree" — while §5.1.1 refuses to create over the
// directory. Without the fallback §5.1.5 would have a state it cannot clear and
// the reader would be left deleting under ~/.cr by hand. The prune still runs
// afterwards, for a registration the directory's old repository may have left.
func removeDirectory(src *Sources, path string) error {
	registered, err := git.WorktreeRegistered(src.RepoDir, path)
	if err != nil {
		return err
	}
	if !registered {
		return removeAsOwnFiles(src)
	}
	refused := git.RemoveWorktree(src.RepoDir, path)
	if refused == nil {
		return nil
	}
	// M-1.4: a registration git lists is not a worktree git will act on.
	// `worktree remove` validates the directory's `.git` file first and
	// refuses one it cannot read — measured, `validation failed, cannot
	// remove working tree: '…/.git' is not a .git file, error code 5`, and
	// `'…/.git' does not exist` for the same directory with the file gone.
	// That is the same position as an unlisted directory, reached by the
	// other road, and it answers the same way: the directory is cr's own
	// files under §2.2's root, so cr removes them and prunes the
	// registration that named them. The refusal is reported only if that
	// fails too, since a failure to remove is the thing worth telling.
	if err := removeAsOwnFiles(src); err != nil {
		return errors.Join(refused, err)
	}
	return nil
}

// removeAsOwnFiles deletes the sandbox directory as what §2.2 makes it — cr's
// own files under the state root — and prunes the registration that named it,
// which is what would otherwise refuse the next `worktree add` at that path.
func removeAsOwnFiles(src *Sources) error {
	if err := src.Layout.RemoveOrphanedSandbox(src.Owner, src.Repo, src.PR); err != nil {
		return err
	}
	return git.PruneWorktrees(src.RepoDir)
}
