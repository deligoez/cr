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
	root := &cobra.Command{
		Use:           "cr",
		Short:         "Code review lifecycle manager for AI coding agents",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
		Args:          cobra.NoArgs,
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

	root.AddCommand(newInitCmd())

	return root
}

// Execute runs the root command and maps errors onto cr's exit codes.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(ExitUsage)
	}
}
