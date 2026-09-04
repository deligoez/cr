package cli

import (
	"github.com/spf13/cobra"
)

// newPostCmd registers §11's `cr post <pr> [--confirm] [--reconcile]`, the
// validation and posting of §8.
//
// The two flags are not a pair of options. §8.5.2 makes `--confirm` the whole
// of the gate — without it the run validates, drafts a payload, and writes
// nothing — and §8.5.3 forbids any setting, variable, field, or alias that
// supplies it implicitly, which is why it is registered here as a flag and
// nowhere else as anything. `--reconcile` is §8.4.4's recovery from an unknown
// outcome: it lists the pull request's reviews, matches §8.4.3's embedded
// payload hash, and either adopts that review as posted or clears
// `post_unresolved` for a retry. §9.3.2 exempts it from the stale-head refusal,
// because it anchors nothing.
//
// Nothing here mints §8.5's confirmation. The gate's own file is the deliberate
// widening of nowrite_test.go's mintSites, and it arrives with the behaviour
// rather than with the surface.
func newPostCmd() *cobra.Command {
	cmd := stubCmd(
		"post "+prPlaceholder,
		"Validate and post the review",
		prArgs(1),
	)
	cmd.Flags().Bool("confirm", false, "perform the network write (§8.5.2)")
	cmd.Flags().Bool("reconcile", false, "resolve an unknown posting outcome (§8.4.4)")
	return cmd
}
