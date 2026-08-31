package git

// AddWorktree checks the repository out at head into path, as a worktree with
// no branch of its own (§5.1.1).
//
// This is the one write in this package, and it is the one write §2.2 permits
// cr anywhere inside the repository under review: the registration git leaves
// under `.git/worktrees/`. Everything else the command touches lands at path,
// which every caller in cr takes from internal/state, so the checkout itself is
// written under the state root.
//
// `--detach` is what holds §5.1.4. head is given as a revision, and a revision
// with `--detach` creates no branch and moves none: the main worktree's HEAD,
// index, and tracked files are not read for this and cannot be written by it.
// Without the flag a head that happened to spell a branch name would be checked
// out as that branch, which is a branch the main worktree may no longer take.
func AddWorktree(repoDir, path, head string) error {
	_, err := run(repoDir, "worktree", "add", "--detach", path, head)
	return err
}
