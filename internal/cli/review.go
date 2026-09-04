package cli

import (
	"github.com/spf13/cobra"
)

// newReviewCmd registers §11's `cr review <pr> [--axis <id>]`, the fan-out of
// §4.6: one prompt per active role and unit, and the cell set §10.2.2 is
// checked against once the roles return.
//
// `--axis` is §4.6.5's two-pass control rather than a filter for convenience.
// The intent axis runs first and produces the mapping, and the remaining axes
// are refused until it exists, so the flag selects which pass is being emitted.
//
// The behaviour belongs to review-prompt-emission and the tasks beside it; what
// is registered here is the shape.
func newReviewCmd() *cobra.Command {
	cmd := stubCmd(
		"review "+prPlaceholder,
		"Emit per-role, per-unit review prompts",
		prArgs(1),
	)
	cmd.Flags().String("axis", "", "emit only this axis's pass (§4.6.5)")
	return cmd
}
