package cli

import (
	"maps"
	"slices"

	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
	"github.com/spf13/cobra"
)

// initResult is what `cr init` has to report: the state tree it prepared.
type initResult struct {
	// Root is ~/.cr, the directory spec/0.1.0.md §2.2 lays out.
	Root string `json:"root"`
}

// Text names the directory, which is the one thing a reader wants confirmed:
// §2.2 puts the tree under $CR_HOME when it is set, so where it landed is not
// always where it was expected to land.
func (r initResult) Text(w *writer) string {
	return "state directory ready at " + w.accent(r.Root)
}

// newInitCmd creates the state tree of spec/0.1.0.md §2.2, writes the profiles
// §2.4.5 ships, and under `--eject-roles` writes the roles §2.5.1 ships.
// Running it again on an existing tree changes nothing, an edited profile or
// role included.
//
// The roles are written only when the flag asks for them. §2.5.4 resolves a
// role per-repository, then globally, then from the built-in layer, so a tree
// with no roles directory of its own is not a tree missing its roles: the
// defaults are in the binary and every review reads them there. Ejecting is how
// a user takes one over, which is why §2.5.2 gives it a flag rather than making
// it what `cr init` always does.
func newInitCmd(out *writer) *cobra.Command {
	var ejectRoles bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create the cr state directory",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
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
			if ejectRoles {
				// Sorted for the reason the profiles are.
				roles := role.Builtins()
				for _, id := range slices.Sorted(maps.Keys(roles)) {
					if err := layout.EnsureRole(id, roles[id]); err != nil {
						return err
					}
				}
			}
			return out.emit(initResult{Root: layout.Root()})
		},
	}
	cmd.Flags().BoolVar(&ejectRoles, "eject-roles", false,
		"Write the built-in roles as editable files")
	return cmd
}
