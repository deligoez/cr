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

// RemoveWorktree deletes the worktree at path and the registration that names
// it (§5.1.5), and then prunes registrations whose directory is already gone.
//
// `--force` is what makes it usable for §5.1.6's recreation: the sandbox being
// removed is exactly the one that failed a cleanliness check, so it is dirty by
// construction, and git refuses a dirty worktree without it. Nothing is lost
// with it — what the sandbox holds is a checkout cr made and a probe's leavings,
// never a user's work, which is the whole reason §2.2 puts it under `~/.cr`.
//
// The prune is not redundant. A directory removed out of band leaves the
// registration behind, and `worktree add` then refuses the path as already
// registered — so a recreation that skipped it would fail on the one case where
// recreation is most obviously needed.
func RemoveWorktree(repoDir, path string) error {
	if _, err := run(repoDir, "worktree", "remove", "--force", "--", path); err != nil {
		return err
	}
	_, err := run(repoDir, "worktree", "prune")
	return err
}

// PruneWorktrees drops the registrations whose directories are gone.
//
// It is RemoveWorktree's second step on its own, for the case where there is no
// directory left to remove: `worktree remove` refuses a path that is not there,
// and the registration it left behind is what would refuse the next
// `worktree add`.
func PruneWorktrees(repoDir string) error {
	_, err := run(repoDir, "worktree", "prune")
	return err
}
