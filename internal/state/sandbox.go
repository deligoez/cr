package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DirSandbox is the name of the probe worktree inside one pull request's state
// directory. §5.1.1 fixes the path, so the name lives here beside the §2.3
// table's file names rather than being spelled at a call site.
const DirSandbox = "sandbox"

// Sandbox is the probe worktree of §5.1.1: a git worktree checked out at the
// pull request head, under that pull request's own state directory.
//
// It is derived here for the reason every other §2.2 path is, and for one more
// of its own. The sandbox is the single place cr runs commands that write —
// §5.1.3's setup, §5.2's test runs, §5.3's mutations — and a caller that joined
// its own segments could aim all of that at a directory the state root does not
// cover. The repository under review is one such directory.
func (l Layout) Sandbox(owner, repo string, pr int) string {
	return filepath.Join(l.PRDir(owner, repo, pr), DirSandbox)
}

// CopyIntoSandbox copies one of §5.1.2's paths from the checkout at repoDir
// into the sandbox, and reports whether the path was there to copy.
//
// An absent path is not a failure. `sandbox.copy` names the working-tree
// artefacts a suite needs and cannot get from a checkout — a `.env`, an
// installed `vendor` tree — and a repository legitimately has neither: the
// laravel-pest profile lists both, and a clone that has never had `composer
// install` run in it holds only one. §5.1.3's setup commands are what produce
// the rest, so refusing here would fail the run before the step that fixes it.
// The answer is returned rather than swallowed, so the caller can report which
// paths were copied and which were not.
//
// The destination is derived from the sandbox path and never from the caller,
// which is what keeps §2.2's root around a copy whose source is a directory in
// the repository under review.
func (l Layout) CopyIntoSandbox(owner, repo string, pr int, repoDir, rel string) (bool, error) {
	if err := l.containRepo(owner, repo); err != nil {
		return false, err
	}
	source := filepath.Join(repoDir, rel)
	if _, err := os.Lstat(source); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("cannot inspect %s: %w", source, err)
	}
	return true, copyTree(source, filepath.Join(l.Sandbox(owner, repo, pr), rel))
}

// RemoveOrphanedSandbox deletes the sandbox directory of one pull request, for
// a caller that has established no git registration answers for it any more.
//
// §5.1.5 removes the worktree through git, and git refuses a directory it no
// longer lists — the repository under review re-cloned under a sandbox leaves
// exactly that. The directory is then only cr's own files under §2.2's root, so
// deleting them is the removal §5.1.5 asks for. The path is derived here, never
// taken from a caller, for the reason Sandbox gives.
func (l Layout) RemoveOrphanedSandbox(owner, repo string, pr int) error {
	if err := l.containRepo(owner, repo); err != nil {
		return err
	}
	path := l.Sandbox(owner, repo, pr)
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("cannot remove %s: %w", path, err)
	}
	return nil
}

// copyTree copies one file, symlink, or directory tree from source to target.
//
// A symlink is recreated rather than followed. `vendor` is full of them, and
// following one would both multiply the bytes copied and turn a link pointing
// out of the tree into a real copy of whatever it pointed at.
func copyTree(source, target string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("cannot inspect %s: %w", source, err)
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		return copyLink(source, target)
	case info.IsDir():
		return copyDir(source, target, info.Mode().Perm())
	case info.Mode().IsRegular():
		return copyFile(source, target, info.Mode().Perm())
	}
	// A socket, a device, or a named pipe. Copying one is meaningless and
	// skipping it silently would leave a sandbox that is quietly missing
	// something the profile asked for, so the run stops and names it.
	return fmt.Errorf("cannot copy %s: it is neither a regular file, a directory, nor a symlink", source)
}

// copyDir recreates a directory and everything under it.
func copyDir(source, target string, perm fs.FileMode) error {
	if err := os.MkdirAll(target, perm); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", target, err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("cannot list %s: %w", source, err)
	}
	for _, entry := range entries {
		if err := copyTree(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyLink recreates a symbolic link, replacing whatever stands at target.
func copyLink(source, target string) error {
	pointsAt, err := os.Readlink(source)
	if err != nil {
		return fmt.Errorf("cannot read the link %s: %w", source, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", filepath.Dir(target), err)
	}
	// A link cannot be created over an existing name, and the sandbox is a
	// checkout that may already hold one of its own.
	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("cannot replace %s: %w", target, err)
	}
	if err := os.Symlink(pointsAt, target); err != nil {
		return fmt.Errorf("cannot link %s: %w", target, err)
	}
	return nil
}

// copyFile copies one regular file, keeping the mode it was read with: an
// executable in a copied `vendor/bin` has to stay executable.
func copyFile(source, target string, perm fs.FileMode) error {
	body, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", source, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, body, perm); err != nil {
		return fmt.Errorf("cannot write %s: %w", target, err)
	}
	return nil
}
