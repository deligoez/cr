package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/sandbox"
	"github.com/deligoez/cr/internal/state"
)

// sandboxCreateResult is what `cr sandbox create` has to report: where the
// worktree landed, what it is checked out at, and what §5.1.2 and §5.1.3 did
// inside it.
//
// The first two are printed rather than confirmed. The path is where every
// later probe and test run happens and is not a path the caller chose, and the
// head is the value §5.1.6 checks the sandbox against before every run — so a
// reader who is told neither cannot tell a sandbox at the round's head from one
// left over from a head that has moved.
type sandboxCreateResult struct {
	// Path is the sandbox worktree of §5.1.1.
	Path string `json:"path"`
	// Head is the pull request head it was checked out at.
	Head string `json:"head"`
	// Copied are the `sandbox.copy` paths §5.1.2 brought in.
	Copied []string `json:"copied"`
	// Absent are the `sandbox.copy` paths the checkout did not hold. A
	// sandbox missing something the profile asked for is a suite that
	// fails for a reason nothing else in the output would explain.
	Absent []string `json:"absent"`
	// Setup are the `sandbox.setup` commands §5.1.3 ran, in order.
	Setup []string `json:"setup"`
}

// Text names the worktree and the revision in it, and then what was done
// inside it. The three lists are printed rather than counted: a reader told
// that two paths were copied still cannot tell which one was not.
func (r *sandboxCreateResult) Text(w *writer) string {
	var out strings.Builder
	fmt.Fprintf(&out, "created %s at %s\n", w.accent(r.Path), r.Head)
	fmt.Fprintf(&out, "  copied %s\n", listed(r.Copied))
	fmt.Fprintf(&out, "  absent %s\n", listed(r.Absent))
	fmt.Fprintf(&out, "  setup  %s", listed(r.Setup))
	return out.String()
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
			// §5.1.2's paths and §5.1.3's commands come from the
			// profile the round resolved, which §3.7 has `cr brief`
			// record. Re-selecting one here could disagree with it,
			// and the sandbox would then be prepared for a profile
			// no role in this round was briefed against.
			steps, file, err := sandboxSteps(layout, round.ProfileID)
			if err != nil {
				return err
			}
			created, err := sandbox.Create(&sandbox.Sources{
				Layout:      layout,
				Owner:       owner,
				Repo:        repo,
				PR:          pr,
				Head:        round.Head,
				RepoDir:     dir,
				Copy:        steps.Copy,
				Setup:       steps.Setup,
				ProfileFile: file,
			})
			if err != nil {
				return err
			}
			return out.emit(&sandboxCreateResult{
				Path:   created.Path,
				Head:   created.Head,
				Copied: created.Copied,
				Absent: created.Absent,
				Setup:  created.Setup,
			})
		},
	}
}

// sandboxSteps reads §5.1.2's copy list and §5.1.3's setup commands out of the
// profile the round resolved, and returns the file they came from beside them.
//
// An empty id is §2.4.4's outcome: no profile matched, so there is nothing to
// copy and nothing to run. The sandbox is still created, because §5.1.1's
// worktree is what every probe and test run of §5 happens in and none of that
// depends on a profile having been found.
func sandboxSteps(l state.Layout, id string) (profile.Sandbox, string, error) {
	resolved, file, err := roundProfile(l, id)
	if err != nil {
		return profile.Sandbox{}, "", err
	}
	return resolved.Sandbox, file, nil
}

// roundProfile loads the profile the round resolved and returns the file it
// came from beside it, so a refusal can name what to open.
//
// An empty id is §2.4.4's outcome: no profile matched. It resolves to the zero
// profile rather than to an error, because what that means differs by caller —
// §5.1.1's worktree is created regardless, while §5.2.1 has no command to run —
// and the empty file name is what says there is nothing to open.
func roundProfile(l state.Layout, id string) (profile.Profile, string, error) {
	if id == "" {
		return profile.Profile{}, "", nil
	}
	file := l.Profile(id)
	resolved, err := profile.Load(file)
	if err != nil {
		return profile.Profile{}, "", err
	}
	return resolved, file, nil
}
