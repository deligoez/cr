package cli

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/state"
	"github.com/spf13/cobra"
)

// configHonesty is the key a configuration listing's honesty disclosures sit
// under, beside the settings. It holds no dot, so no setting of §2.7's table
// can ever be spelled the same, and it is present only when there is something
// to disclose: every other key of the listing is a setting.
const configHonesty = "honesty"

// configResult is the effective configuration of spec/0.1.0.md §2.7, keyed by
// the flat dotted names that section gives its settings, and carrying
// configHonesty when a layer was not consulted.
type configResult map[string]any

// Text lists every setting, one per line, in key order. The order is sorted
// rather than incidental so two runs can be diffed against each other, which is
// most of what reading the effective configuration is for.
func (r configResult) Text(w *writer) string {
	lines := make([]string, 0, len(r))
	for _, key := range settingKeys(r) {
		lines = append(lines, fmt.Sprintf("%s = %v", w.accent(key), r[key]))
	}
	notes, _ := r[configHonesty].([]string)
	return strings.Join(lines, "\n") + w.disclose("\n", "", notes...)
}

// settingKeys are a listing's keys in order, without configHonesty.
func settingKeys[V any](listing map[string]V) []string {
	return slices.DeleteFunc(slices.Sorted(maps.Keys(listing)), func(key string) bool {
		return key == configHonesty
	})
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
	// Inert says why a setting is resolved and not in force, and is absent
	// for every setting that is. The row stays, because §2.7 annotates
	// every setting; what the annotation adds is that this one is read by
	// nothing. Measured 2026-09-22 against deligoez/cr-qa: under
	// `intent.tracker: github` v0.6.0 listed the Jira key pattern as if it
	// held, while §3.2 held keys to owner.repo#n.
	Inert string `json:"inert,omitempty"`
}

// inertUnderGitHub are the settings the GitHub tracker reads none of, with
// the reason each is inert: cr forms the key itself and reads the issue
// through gh.
var inertUnderGitHub = map[string]string{
	"intent.key_pattern": "not in force: intent.tracker is github, and §3.2 holds keys to " +
		intent.GitHubKeyPattern,
	"intent.cmd": "not in force: intent.tracker is github, and §3.1.8 reads the issue through gh",
}

// resolvedConfigResult is §2.7's annotated configuration, keyed by the same
// flat dotted names configResult uses: a resolvedSetting under each, and
// configHonesty when a layer was not consulted.
type resolvedConfigResult map[string]any

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
	for _, key := range settingKeys(r) {
		setting, _ := r[key].(resolvedSetting)
		from := setting.From
		if setting.Source != "" {
			from += " " + setting.Source
		}
		if setting.Inert != "" {
			from += "; " + setting.Inert
		}
		lines = append(lines, fmt.Sprintf("%s = %v  (%s)", w.accent(key), setting.Value, from))
	}
	notes, _ := r[configHonesty].([]string)
	return strings.Join(lines, "\n") + w.disclose("\n", "", notes...)
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
	github := resolved.String(intent.TrackerSetting) == intent.TrackerGitHub
	out := make(resolvedConfigResult, len(origins))
	for key, value := range resolved.Map() {
		setting := resolvedSetting{
			Value:  value,
			From:   origins[key].From,
			Source: origins[key].Source,
		}
		if github {
			setting.Inert = inertUnderGitHub[key]
		}
		out[key] = setting
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
//
// The per-repository layer is located the way a pull-request command locates
// it, through repoOf: `--repo` when given, and otherwise the repository
// detected from the checkout's one GitHub remote. So the value and the layer
// printed inside a clone are the ones `cr brief` there reads. Where detection
// names no repository, the listing is still the effective configuration of
// every other layer, and it says the per-repository layer was not consulted and
// why, rather than printing a value a pull-request command would not use as if
// it were the whole answer.
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
			var notes []string
			owner, name, err := repoOf(cmd)
			var undetected *RepositoryDetectionError
			switch {
			case errors.As(err, &undetected):
				notes = append(notes, fmt.Sprintf("the %s layer of §2.7 was not consulted: %v; "+
					"run cr config inside a clone of the repository, or pass --repo <owner/repo>",
					config.LayerRepoConfig, undetected))
			case err != nil:
				return err
			default:
				sources.RepoConfig = layout.RepoConfig(owner, name)
			}
			resolved, err := config.Resolve(sources)
			if err != nil {
				return err
			}
			if annotate {
				listing := annotated(resolved)
				if len(notes) > 0 {
					listing[configHonesty] = notes
				}
				return out.emit(listing)
			}
			listing := configResult(resolved.Map())
			if len(notes) > 0 {
				listing[configHonesty] = notes
			}
			return out.emit(listing)
		},
	}
	cmd.Flags().Bool("resolved", false, "annotate each setting with the layer it came from")
	return cmd
}

// splitRepo reads an owner/repo argument, which the per-repository layer of
// §2.7 needs before it can be located.
//
// Each half becomes one directory of §2.2's tree, so each has to be one path
// segment: a `..` or a separator in either would aim cr's state somewhere the
// argument does not name. Audit round 1 measured `--repo ../..` reaching the
// state package unrefused. The refusal is §11.2's code 2 — the command line is
// what is wrong — and internal/state refuses a path outside the root again for
// any caller that did not come through here.
//
// Both halves are folded to lower case and a `.git` suffix is dropped, because
// GitHub names one repository by every spelling of its slug and the state tree
// must too. Measured before the fold: `--repo Deligoez/CR-QA-GO` found a round
// opened as `deligoez/cr-qa-go` only because macOS's filesystem folds case, so
// a case-sensitive one could not, and `--repo owner/repo.git` found none.
func splitRepo(repo string) (owner, name string, err error) {
	owner, name, ok := strings.Cut(repo, "/")
	owner, name = strings.ToLower(owner), strings.ToLower(strings.TrimSuffix(name, ".git"))
	if !ok || !pathSegment(owner) || !pathSegment(name) {
		return "", "", fmt.Errorf("invalid repository %q: pass it as owner/repo, "+
			"where neither half is . or .. or holds a separator", repo)
	}
	return owner, name, nil
}

// pathSegment reports whether s names exactly one directory entry.
func pathSegment(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\`)
}
