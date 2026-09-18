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
// the repository under review. The sandbox is then opened as an os.Root and
// every write goes through it: see copyTree for what that buys and why joining
// the path is not enough.
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
	sandbox := l.Sandbox(owner, repo, pr)
	// A caller reaches here with the worktree already added, so the
	// directory is there; it is created for the one that has not, because
	// a root cannot be opened on a path that does not exist and a copy
	// into a sandbox is not the place to decide that one is missing.
	if err := os.MkdirAll(sandbox, dirPerm); err != nil {
		return false, fmt.Errorf("cannot create directory %s: %w", sandbox, err)
	}
	root, err := os.OpenRoot(sandbox)
	if err != nil {
		return false, fmt.Errorf("cannot open the sandbox %s: %w", sandbox, err)
	}
	// The close is deferred and its error dropped: every write below has
	// already reported its own, and a directory handle closes nothing a
	// caller could act on.
	defer func() { _ = root.Close() }()
	within := filepath.Clean(rel)
	if err := plantDirs(root, filepath.Dir(within)); err != nil {
		return false, err
	}
	return true, copyTree(root, source, within)
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

// copyTree copies one file, symlink, or directory tree from source to the path
// rel inside root, which is the sandbox.
//
// Every write goes through the root rather than through a joined path, and that
// is what keeps §5.1.2 inside §2.2's tree. A `sandbox.copy` entry is held to a
// path inside the checkout when the profile is read, which settles what the
// profile may name and nothing about what the head under review checks out
// there: a branch may perfectly well track `.env`, or a directory a copied path
// descends through, as a link to somewhere else on the machine. Joining and
// writing would follow it, and the user's own `.env` — credentials included —
// would land outside ~/.cr entirely, which invariant 2 permits nowhere. A root
// refuses that write instead (`openat: path escapes from parent`, measured), so
// what the head put there is replaced by what the checkout holds, which is what
// §5.1.2 asks for anyway.
//
// A symlink is recreated rather than followed. `vendor` is full of them, and
// following one would both multiply the bytes copied and turn a link pointing
// out of the tree into a real copy of whatever it pointed at. Creating one is
// still allowed through the root: the link is data, and nothing is written
// through it here.
func copyTree(root *os.Root, source, rel string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("cannot inspect %s: %w", source, err)
	}
	if err := clearTarget(root, rel, info.IsDir()); err != nil {
		return err
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		return copyLink(root, source, rel)
	case info.IsDir():
		return copyDir(root, source, rel, info.Mode().Perm())
	case info.Mode().IsRegular():
		return copyFile(root, source, rel, info.Mode().Perm())
	}
	// A socket, a device, or a named pipe. Copying one is meaningless and
	// skipping it silently would leave a sandbox that is quietly missing
	// something the profile asked for, so the run stops and names it.
	return fmt.Errorf("cannot copy %s: it is neither a regular file, a directory, nor a symlink", source)
}

// plantDirs makes every component of rel a real directory inside root.
//
// It is copyTree's question asked of the parents rather than of the entry:
// `config/local.php` is a path inside the checkout whether the head tracks
// `config` as a directory or as a link pointing somewhere else, and a root
// refuses to write under the link rather than following it. So a component that
// is not a directory is replaced by one, and the copy lands where the caller
// asked for it.
func plantDirs(root *os.Root, rel string) error {
	if rel == "." || rel == string(filepath.Separator) {
		return nil
	}
	if err := plantDirs(root, filepath.Dir(rel)); err != nil {
		return err
	}
	if err := clearTarget(root, rel, true); err != nil {
		return err
	}
	if err := root.Mkdir(rel, dirPerm); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("cannot create directory %s in the sandbox: %w", rel, err)
	}
	return nil
}

// clearTarget removes whatever stands at rel inside root, and leaves a
// directory standing when a directory is what is about to be written there.
//
// The removal is what makes a copy a replacement. The sandbox is a checkout of
// the head, so a copied path often arrives on top of a file git just wrote; a
// link cannot be created over an existing name at all, and a link left standing
// is a name every later write would resolve through. Lstat and RemoveAll both
// act on the name rather than on what it points at, so a link is removed and its
// target is not.
func clearTarget(root *os.Root, rel string, keepDir bool) error {
	info, err := root.Lstat(rel)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("cannot inspect %s in the sandbox: %w", rel, err)
	case keepDir && info.IsDir():
		return nil
	}
	if err := root.RemoveAll(rel); err != nil {
		return fmt.Errorf("cannot replace %s in the sandbox: %w", rel, err)
	}
	return nil
}

// copyDir recreates a directory and everything under it.
func copyDir(root *os.Root, source, rel string, perm fs.FileMode) error {
	if err := root.Mkdir(rel, perm); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("cannot create directory %s in the sandbox: %w", rel, err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("cannot list %s: %w", source, err)
	}
	for _, entry := range entries {
		if err := copyTree(root, filepath.Join(source, entry.Name()), filepath.Join(rel, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyLink recreates a symbolic link.
func copyLink(root *os.Root, source, rel string) error {
	pointsAt, err := os.Readlink(source)
	if err != nil {
		return fmt.Errorf("cannot read the link %s: %w", source, err)
	}
	if err := root.Symlink(pointsAt, rel); err != nil {
		return fmt.Errorf("cannot link %s in the sandbox: %w", rel, err)
	}
	return nil
}

// copyFile copies one regular file, keeping the mode it was read with: an
// executable in a copied `vendor/bin` has to stay executable.
func copyFile(root *os.Root, source, rel string, perm fs.FileMode) error {
	body, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", source, err)
	}
	if err := root.WriteFile(rel, body, perm); err != nil {
		return fmt.Errorf("cannot write %s in the sandbox: %w", rel, err)
	}
	return nil
}
