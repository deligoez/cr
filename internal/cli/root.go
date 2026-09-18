// Package cli wires the cr command surface.
package cli

import (
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// version is the tag a release build injects via
// -ldflags "-X github.com/deligoez/cr/internal/cli.version=<tag>" (§14.6). It is
// empty in every other build, and versionOf says what `cr --version` prints
// then.
var version string

func newRootCmd() *cobra.Command {
	// One writer for the whole tree. §12.1's decision is settled on it once,
	// before any command runs, so every command in the tree emits through
	// the same answer rather than reaching its own.
	out := &writer{}
	root := &cobra.Command{
		Use:           "cr",
		Short:         "Code review lifecycle manager for AI coding agents",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       versionOf(version, debug.ReadBuildInfo),
		Args:          cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := refuseProtected(cmd); err != nil {
				return err
			}
			return out.settle(cmd)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetVersionTemplate("cr version {{.Version}}\n")

	root.PersistentFlags().Bool("json", false, "force JSON output")
	root.PersistentFlags().Bool("compact", false, "minimal JSON output")
	// `--quiet` suppresses informational messages and nothing else (§11.1).
	// The writer reads it in one place, informational, which takes a test
	// run's live echo to standard error; the seven disclosures §11.1 names
	// are rendered through writer.disclose, which reads no flag, and a
	// failure through reportFailure, which is given none. §12.1's document
	// is the same with and without it.
	// TestEveryDisclosureAResultPrintsGoesThroughTheWriter holds the split.
	root.PersistentFlags().Bool("quiet", false, "suppress informational messages")
	root.PersistentFlags().Bool("no-color", false, "disable colored output")
	root.PersistentFlags().String("repo", "", "override repository detection (owner/repo)")

	root.AddCommand(newInitCmd(out))
	root.AddCommand(newConfigCmd(out))
	root.AddCommand(newNoteCmd(out))
	root.AddCommand(newAnswerCmd(out))
	root.AddCommand(newContextCmd(out))
	root.AddCommand(newRecordCmd(out))
	root.AddCommand(newClaimsCmd(out))
	root.AddCommand(newCellsCmd(out))
	root.AddCommand(newProposalsCmd(out))
	root.AddCommand(newMapCmd(out))
	root.AddCommand(newBriefCmd(out))
	root.AddCommand(newSandboxCmd(out))
	root.AddCommand(newTestCmd(out))
	root.AddCommand(newProbeCmd(out))
	root.AddCommand(newDraftCmd(out))
	root.AddCommand(newTriageCmd(out))
	root.AddCommand(newStatusCmd(out))
	// The rest of §11's table. Each was registered with its argument shape
	// and its flags before its behaviour was built, because the surface is
	// the half a caller writes a script against: a command that is absent is
	// indistinguishable from a command that is misspelled, and both come
	// back as §11.2's code 2. Every one of them now runs.
	root.AddCommand(newReviewCmd(out))
	root.AddCommand(newMergeCmd(out))
	root.AddCommand(newPostCmd(out))
	root.AddCommand(newStatsCmd(out))
	root.AddCommand(newWaiversCmd(out))
	root.AddCommand(newRulesCmd(out))

	return root
}

// refuseProtected makes §2.7's refusal of a name addressing a protected
// decision for every command in the tree, before its work: a CR_ variable, a
// key of either config layer, and a field of a profile file. The refusal cannot
// wait for a command to resolve its configuration or load its profile, because
// `cr record`, `cr merge` and `cr waivers list` do neither, and QA found them
// accepting all three in silence (D-S05-5, D-V1a-6).
//
// The per-repository layer is the one repoOf locates, as for `cr config`.
// Where the state root, or the repository, cannot be located, that part is left
// to the command: one that needs it refuses by its own name, and one that does
// not, such as `cr init` outside a clone, is not refused for it.
func refuseProtected(cmd *cobra.Command) error {
	if err := config.CheckEnviron(os.Environ()); err != nil {
		return err
	}
	layout, layoutErr := state.Default()
	if layoutErr != nil {
		return nil
	}
	if err := config.CheckFile(layout.Config()); err != nil {
		return err
	}
	if owner, repo, repoErr := repoOf(cmd); repoErr == nil {
		if err := config.CheckFile(layout.RepoConfig(owner, repo)); err != nil {
			return err
		}
	}
	return profile.CheckDir(layout.ProfilesDir())
}

// Execute runs the root command and maps errors onto cr's exit codes.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		// A failure to report the failure has nowhere left to go; the exit
		// code below still says what happened.
		_ = reportFailure(os.Stdout, os.Stderr, os.Args[1:], err)
		os.Exit(exitCodeFor(err))
	}
}
