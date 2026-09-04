package cli

import (
	"github.com/spf13/cobra"
)

// newStatsCmd registers §11's `cr stats --repo <owner/repo>`: §7.3.2's per
// class and per rule counts, §7.3.3's newly seen classes, and §7.3.4's
// demotion and §7.3.6's volume candidates.
//
// It is scoped to the repository and not to a pull request, because §7.3.4
// computes the demotion rate over that repository's whole `triage.ndjson` —
// a rate over one pull request's events would be a number with no sample
// behind it. `--repo` is §11.1's global flag, registered on the root command.
func newStatsCmd() *cobra.Command {
	return stubCmd(
		"stats",
		"Report triage statistics, demotion and volume candidates",
		cobra.NoArgs,
	)
}
