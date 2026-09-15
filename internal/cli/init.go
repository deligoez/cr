package cli

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
	"github.com/spf13/cobra"
)

// initResult is what `cr init` has to report: the state tree it prepared, and
// what it did to the profiles already in it.
type initResult struct {
	// Root is ~/.cr, the directory spec/0.1.0.md §2.2 lays out.
	Root string `json:"root"`
	// Updated are the profile files that held an earlier release's shipped
	// profile, byte for byte, and now hold this release's.
	Updated []string `json:"updated"`
	// Honesty names each shipped profile's file that matches no shipped
	// version and was therefore left as it was, while the shipped profile
	// may have changed under it.
	Honesty []string `json:"honesty"`
}

// Text names the directory, which is the one thing a reader wants confirmed:
// §2.2 puts the tree under $CR_HOME when it is set, so where it landed is not
// always where it was expected to land. Every updated profile is named after
// it, because a file cr rewrote is one the reader did not.
func (r initResult) Text(w *writer) string {
	var out strings.Builder
	out.WriteString("state directory ready at " + w.accent(r.Root))
	for _, file := range r.Updated {
		out.WriteString("\n  updated " + file)
	}
	return out.String() + w.disclose("\n", "", r.Honesty...)
}

// newInitCmd creates the state tree of spec/0.1.0.md §2.2, writes the profiles
// §2.4.5 ships, and under `--eject-roles` writes the roles §2.5.1 ships.
// Running it again on an existing tree changes no edited profile or role; it
// updates only a profile file still holding an earlier release's shipped bytes,
// as refreshProfiles says.
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
			profiles, err := refreshProfiles(layout)
			if err != nil {
				return err
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
			profiles.Root = layout.Root()
			return out.emit(profiles)
		},
	}
	cmd.Flags().BoolVar(&ejectRoles, "eject-roles", false,
		"Write the built-in roles as editable files")
	return cmd
}

// refreshProfiles writes every shipped profile into the profiles directory,
// and brings up to date each file still holding an earlier release's shipped
// profile byte for byte.
//
// A file equal to an earlier release's profile carries no edit, so replacing
// it loses nothing, and it is how a repair to a shipped profile reaches a user
// who ran `cr init` before the repair: laravel-pest's `sandbox.copy` gained
// `.env.testing` after a sandbox run was measured reaching the developer's own
// database, and a file `cr init` never touched again would keep the old list.
// Any other file is the user's and stays as it is. It is named, because the
// shipped profile may have changed while that copy did not.
func refreshProfiles(l state.Layout) (initResult, error) {
	result := initResult{Updated: make([]string, 0), Honesty: make([]string, 0)}
	shipped := profile.Builtins()
	// Sorted, so a failure part-way through leaves the same profiles behind
	// on every run.
	for _, id := range slices.Sorted(maps.Keys(shipped)) {
		var standing profile.Standing
		outcome, err := l.RefreshProfile(id, shipped[id], func(onDisk []byte) bool {
			standing = profile.StandingOf(id, onDisk)
			return standing.Stale()
		})
		if err != nil {
			return initResult{}, err
		}
		switch {
		case outcome == state.ProfileReplaced:
			result.Updated = append(result.Updated, l.Profile(id))
		case outcome == state.ProfileKept && !standing.Current:
			result.Honesty = append(result.Honesty, fmt.Sprintf(
				"%s matches no version of the %s profile cr has shipped, so cr init left it as it is; "+
					"the shipped profile may have changed since, and cr init with CR_HOME set to an empty directory writes it for comparison",
				l.Profile(id), id))
		}
	}
	return result, nil
}
