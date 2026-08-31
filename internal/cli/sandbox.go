package cli

import (
	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/sandbox"
	"github.com/deligoez/cr/internal/state"
)

// sandboxCreateResult is what `cr sandbox create` has to report: where the
// worktree landed and what it is checked out at.
//
// Both are printed rather than confirmed. The path is where every later probe
// and test run happens and is not a path the caller chose, and the head is the
// value §5.1.6 checks the sandbox against before every run — so a reader who is
// told neither cannot tell a sandbox at the round's head from one left over
// from a head that has moved.
type sandboxCreateResult struct {
	// Path is the sandbox worktree of §5.1.1.
	Path string `json:"path"`
	// Head is the pull request head it was checked out at.
	Head string `json:"head"`
}

// Text names the worktree and the revision in it.
func (r *sandboxCreateResult) Text(w *writer) string {
	return "created " + w.accent(r.Path) + " at " + r.Head
}

// newSandboxCmd groups the sandbox commands of §11. It runs nothing itself, so
// an invocation naming no subcommand prints the help rather than creating a
// worktree nobody asked for.
func newSandboxCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sandbox",
		Short: "Manage the probe worktree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newSandboxCreateCmd(out))
	return cmd
}

// newSandboxCreateCmd creates the probe worktree at the pull request head
// (§11, §5.1.1).
//
// The head comes from meta.json through Briefed rather than from the command
// line or from the checkout's own HEAD. §5.1.1 says the PR head, §3.7 makes
// `cr brief` the one writer of it, and §5.1.6 compares the sandbox's HEAD
// against it before every run — so a sandbox created at anything else would
// fail that check on its first use, having already run §5.1.3's setup.
func newSandboxCreateCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "create " + prPlaceholder,
		Short: "Create the probe worktree at the pull request head",
		Args:  prArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pr, err := parsePR(args[0])
			if err != nil {
				return err
			}
			owner, repo, err := repoOf(cmd)
			if err != nil {
				return err
			}
			layout, err := state.Default()
			if err != nil {
				return err
			}
			round, err := layout.Briefed(owner, repo, pr)
			if err != nil {
				return err
			}
			// The repository under review is the directory cr was run
			// from, as it is for `cr brief`: §11.1 makes `--repo` the
			// override for repository detection and not for the
			// checkout the worktree is registered in.
			dir, err := repoDir()
			if err != nil {
				return err
			}
			created, err := sandbox.Create(&sandbox.Sources{
				Layout:  layout,
				Owner:   owner,
				Repo:    repo,
				PR:      pr,
				Head:    round.Head,
				RepoDir: dir,
			})
			if err != nil {
				return err
			}
			return out.emit(&sandboxCreateResult{Path: created.Path, Head: created.Head})
		},
	}
}
