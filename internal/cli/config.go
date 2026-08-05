package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/state"
	"github.com/spf13/cobra"
)

// newConfigCmd prints the effective configuration of spec/0.1.0.md §2.7,
// resolved at read time from every layer.
func newConfigCmd() *cobra.Command {
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
			encoded, err := json.MarshalIndent(resolved.Map(), "", "  ")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
			return err
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
