package cli

import (
	"github.com/spf13/cobra"
)

// Every §11 row is built. The stub mechanism that registered a row's argument
// shape ahead of its behaviour left with the last stub, `cr rules list`; what
// remains here is the group command the grouped rows hang from.

// groupCmd registers a §11 row that is a group of subcommands rather than a
// command of its own. It runs nothing itself, so an invocation naming no
// subcommand prints the help rather than doing something the user did not ask
// for — the shape `cr claims` and `cr sandbox` already take.
func groupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
}
