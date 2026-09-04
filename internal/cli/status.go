package cli

import (
	"github.com/spf13/cobra"
)

// newStatusCmd registers §11's `cr status <pr>`: §10.1's coverage report,
// §9.1's record states, and §10.2's completeness verdict with the exact reason
// when it is false.
//
// It carries no flag of its own. §10.2 has the verdict printed together with
// every lens of §4.5.4 that did not run, and §11.1 forbids `--quiet` to
// suppress that report, so there is nothing here for a flag to select: what
// `cr status` prints is fixed by the round rather than by the invocation.
func newStatusCmd() *cobra.Command {
	return stubCmd(
		"status "+prPlaceholder,
		"Report coverage, record states, and completeness",
		prArgs(1),
	)
}
