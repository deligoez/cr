package state

import "path/filepath"

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
