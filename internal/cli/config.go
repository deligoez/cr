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

// resolvedSetting is one setting as `cr config --resolved` reports it: the
// value in force, and §2.7's layer that supplied it.
//
// The source sits beside the layer rather than replacing it, because the two
// answer different questions and only one of them is always answerable. The
// layer says where in §2.7's order the value was settled, which is what
// explains why a value a reader can see in a file is not the one in force; the
// source says which file or variable to open, and the two layers that are not a
// place — the built-in defaults and the command line — have none.
type resolvedSetting struct {
	// Value is the value in force, exactly as `cr config` prints it.
	Value any `json:"value"`
	// From is the layer, one of §2.7's five, per config.Layer*. It is
	// spelled this way for the reason config.Origin's own field is:
	// internal/role fences the bare identifier `Layer` to its own package.
	From string `json:"layer"`
	// Source is the config file's path or the environment variable's
	// name, and empty for the two layers that are not a place.
	Source string `json:"source"`
}

// resolvedConfigResult is §2.7's annotated configuration, keyed by the same
// flat dotted names configResult uses.
type resolvedConfigResult map[string]resolvedSetting

// Text lists every setting with its value and where that value came from, in
// key order, for the reason configResult sorts: two runs are diffed against
// each other, and that is most of what reading the effective configuration is
// for.
//
// The annotation names the file or the variable wherever there is one, because
// §2.7's layers are a rank and a rank is not somewhere a reader can go. A
// setting no layer touched says which layer that was and stops, since the
// built-in defaults are in cr itself.
func (r resolvedConfigResult) Text(w *writer) string {
	lines := make([]string, 0, len(r))
	for _, key := range slices.Sorted(maps.Keys(r)) {
		setting := r[key]
		from := setting.From
		if setting.Source != "" {
			from += " " + setting.Source
		}
		lines = append(lines, fmt.Sprintf("%s = %v  (%s)", w.accent(key), setting.Value, from))
	}
	return strings.Join(lines, "\n")
}

// annotated joins the resolved values to the layers that supplied them, which
// is the whole of what `--resolved` adds.
//
// Both maps are the resolution's own and are keyed alike, so every key of one
// is a key of the other; the layer is read without a comma-ok for that reason,
// and a key that somehow carried none would be reported with an empty layer
// rather than silently dropped from the listing.
func annotated(resolved config.Config) resolvedConfigResult {
	origins := resolved.Origins()
	out := make(resolvedConfigResult, len(origins))
	for key, value := range resolved.Map() {
		out[key] = resolvedSetting{
			Value:  value,
			From:   origins[key].From,
			Source: origins[key].Source,
		}
	}
	return out
}

// newConfigCmd prints the effective configuration of spec/0.1.0.md §2.7,
// resolved at read time from every layer.
//
// `--resolved` is §11's second form of this row: the same settings, each with
// the layer that supplied it. It is one resolution read twice rather than a
// second pass over the layers — config.Config carries the provenance it settled
// — so the annotated listing cannot name a layer for a value the plain listing
// does not print.
func newConfigCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Print the effective configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			annotate, err := cmd.Flags().GetBool("resolved")
			if err != nil {
				return err
			}
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
			if annotate {
				return out.emit(annotated(resolved))
			}
			return out.emit(configResult(resolved.Map()))
		},
	}
	cmd.Flags().Bool("resolved", false, "annotate each setting with the layer it came from")
	return cmd
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
