package cli

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/state"
	"github.com/spf13/cobra"
)

// configResult is the effective configuration of spec/0.1.0.md §2.7, keyed by
// the flat dotted names that section gives its settings.
type configResult map[string]any

// Text lists every setting, one per line, in key order. The order is sorted
// rather than incidental so two runs can be diffed against each other, which is
// most of what reading the effective configuration is for.
func (r configResult) Text(w *writer) string {
	lines := make([]string, 0, len(r))
	for _, key := range slices.Sorted(maps.Keys(r)) {
		lines = append(lines, fmt.Sprintf("%s = %v", w.accent(key), r[key]))
	}
	return strings.Join(lines, "\n")
}

// newConfigCmd prints the effective configuration of spec/0.1.0.md §2.7,
// resolved at read time from every layer.
func newConfigCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print the effective configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			layout, err := state.Default()
			if err != nil {
				return err
			}
			// No v0.1 flag addresses a setting, so the flag layer is empty
			// here. A command that grows one passes it in Sources.Flags.
			sources := config.Sources{
				Environ:      os.Environ(),
				GlobalConfig: layout.Config(),
			}
			repo, err := cmd.Flags().GetString("repo")
			if err != nil {
				return err
			}
			if repo != "" {
				owner, name, err := splitRepo(repo)
				if err != nil {
					return err
				}
				sources.RepoConfig = layout.RepoConfig(owner, name)
			}
			resolved, err := config.Resolve(sources)
			if err != nil {
				return err
			}
			return out.emit(configResult(resolved.Map()))
		},
	}
}

// splitRepo reads an owner/repo argument, which the per-repository layer of
// §2.7 needs before it can be located.
func splitRepo(repo string) (owner, name string, err error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return "", "", fmt.Errorf("invalid repository %q: pass it as owner/repo", repo)
	}
	return owner, name, nil
}
