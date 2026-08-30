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
	root.PersistentFlags().Bool("quiet", false, "suppress informational messages")
	root.PersistentFlags().Bool("no-color", false, "disable colored output")
	root.PersistentFlags().String("repo", "", "override repository detection (owner/repo)")

	root.AddCommand(newInitCmd(out))
	root.AddCommand(newConfigCmd(out))
	root.AddCommand(newNoteCmd(out))
	root.AddCommand(newAnswerCmd(out))
	root.AddCommand(newContextCmd(out))

	return root
}

// Execute runs the root command and maps errors onto cr's exit codes.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitCodeFor(err))
	}
}
