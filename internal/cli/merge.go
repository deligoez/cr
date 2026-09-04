package cli

import (
	"github.com/spf13/cobra"
)

// newMergeCmd registers §11's
// `cr merge <files...> -o <out> --repo <owner/repo> --pr <n>`, the merge of
// §6.5: the per-role NDJSON files are deduplicated per §6.4 and the counts are
// reported by role, axis, severity, and grade.
//
// It takes the pull request as a flag rather than as the positional every other
// PR-scoped command uses, because its positionals are the files being merged
// and §6.5.1 states the invocation that way. The repository and the pull
// request are both required there for a reason that is not bookkeeping: §6.4.2
// needs the resolved role corpus and §6.4.4 the repository-wide waivers, and
// §7.4.1 scopes `not-here` waivers to the pull request — a merge that could not
// read them would resurface exactly what the reviewer set aside. `--repo` is
// §11.1's global flag and is registered on the root command.
//
// The behaviour belongs to merge-command and merge-counts-reporting; what is
// registered here is the shape.
func newMergeCmd() *cobra.Command {
	cmd := stubCmd(
		"merge <files...>",
		"Merge and deduplicate per-role findings",
		cobra.MinimumNArgs(1),
	)
	cmd.Flags().StringP("output", "o", "", "write the merged findings to this file")
	cmd.Flags().Int("pr", 0, "the pull request the findings belong to")
	return cmd
}
