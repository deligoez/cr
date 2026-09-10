// Package cli wires the cr command surface.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is injected at build time via -ldflags.
var version = "dev"

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
		Version:       version,
		Args:          cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return out.settle(cmd)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetVersionTemplate("cr version {{.Version}}\n")

	root.PersistentFlags().Bool("json", false, "force JSON output")
	root.PersistentFlags().Bool("compact", false, "minimal JSON output")
	// `--quiet` suppresses nothing, which is the decision and not an
	// omission. §11.1 gives it informational messages and then names seven
	// disclosures it may never touch; cr prints no informational message
	// today, since every byte on stdout is a command's own result and §12.1
	// settles that shape from stdout and `--json` alone. A suppression
	// written now would have nothing to suppress and one thing to break —
	// it would land before the exempt writer that quiet-honesty-exemptions
	// owns, and the seven would go out with the informational messages.
	// TestNoSuppressionUnderQuietArrivesBeforeItsExemption holds the order.
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
	root.AddCommand(newMapCmd(out))
	root.AddCommand(newBriefCmd(out))
	root.AddCommand(newSandboxCmd(out))
	root.AddCommand(newTestCmd(out))
	root.AddCommand(newProbeCmd(out))
	root.AddCommand(newDraftCmd(out))
	// The rest of §11's table. These carry their argument shape and their
	// flags and refuse to run, because the surface is the half a caller
	// writes a script against and it is worth completing before the
	// behaviour: a command that is absent is indistinguishable from a
	// command that is misspelled, and both come back as §11.2's code 2.
	root.AddCommand(newReviewCmd(out))
	root.AddCommand(newMergeCmd())
	root.AddCommand(newPostCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newStatsCmd())
	root.AddCommand(newWaiversCmd())
	root.AddCommand(newRulesCmd(out))

	return root
}

// Execute runs the root command and maps errors onto cr's exit codes.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitCodeFor(err))
	}
}
