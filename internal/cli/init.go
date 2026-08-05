package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/state"
	"github.com/spf13/cobra"
)

// newInitCmd creates the state tree of spec/0.1.0.md §2.2. Running it again on
// an existing tree changes nothing.
func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create the cr state directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			layout, err := state.Default()
			if err != nil {
				return err
			}
			if err := layout.Init(); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "state directory ready at %s\n", layout.Root())
			return err
		},
	}
}
