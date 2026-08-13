package cli

import (
	"fmt"
	"maps"
	"slices"

	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
	"github.com/spf13/cobra"
)

// newInitCmd creates the state tree of spec/0.1.0.md §2.2 and writes the
// profiles §2.4.5 ships. Running it again on an existing tree changes nothing,
// an edited profile included.
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
			// Sorted, so a failure part-way through leaves the same
			// profiles behind on every run.
			shipped := profile.Builtins()
			for _, id := range slices.Sorted(maps.Keys(shipped)) {
				if err := layout.EnsureProfile(id, shipped[id]); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "state directory ready at %s\n", layout.Root())
			return err
		},
	}
}
