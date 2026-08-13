package cli

import (
	"maps"
	"slices"

	"github.com/deligoez/cr/internal/profile"
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

// newInitCmd creates the state tree of spec/0.1.0.md §2.2 and writes the
// profiles §2.4.5 ships. Running it again on an existing tree changes nothing,
// an edited profile included.
func newInitCmd(out *writer) *cobra.Command {
	return &cobra.Command{
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
			return out.emit(initResult{Root: layout.Root()})
		},
	}
}
