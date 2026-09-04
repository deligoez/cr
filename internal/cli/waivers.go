package cli

import (
	"github.com/spf13/cobra"
)

// newWaiversCmd groups §11's waiver commands. §7.4.4 requires both scopes to be
// readable and removable, so neither disposition is a one-way door, and these
// two are that door in the other direction.
func newWaiversCmd() *cobra.Command {
	cmd := groupCmd("waivers", "Inspect and edit waivers in either scope")
	cmd.AddCommand(newWaiversListCmd(), newWaiversRemoveCmd())
	return cmd
}

// newWaiversListCmd registers §11's
// `cr waivers list --repo <owner/repo> [--pr <n>]`, which §7.4.7 has print
// active waivers with their scope and disposition.
//
// `--pr` selects how many of §7.4.4's two files are read, not which pull
// request a single file is filtered to: with it, both the repository-wide file
// and that pull request's `waivers.ndjson`; without it, the repository-wide
// file alone.
//
// The behaviour belongs to waivers-list-command; what is registered here is the
// shape.
func newWaiversListCmd() *cobra.Command {
	cmd := stubCmd("list", "Print active waivers with their scope and disposition", cobra.NoArgs)
	cmd.Flags().Int("pr", 0, "also read this pull request's waivers (§7.4.4)")
	return cmd
}

// newWaiversRemoveCmd registers §11's
// `cr waivers remove --repo <owner/repo> [--pr <n>]`, which §7.4.7 has delete
// one waiver from either scope.
//
// It takes the waiver id as a positional even though §11's row does not name
// one, for the reason `cr brief` registers `--intent-file`: §7.4.7 writes the
// invocation as `cr waivers remove <id> --repo <owner/repo> [--pr <number>]`,
// and a surface that followed §11 into the gap would be a `remove` with nothing
// to remove. TestTheCommandSurfaceIsTheSpecTable records it as a deliberate
// addition rather than letting it pass as an oversight.
//
// The behaviour belongs to waivers-remove-command; what is registered here is
// the shape.
func newWaiversRemoveCmd() *cobra.Command {
	cmd := stubCmd("remove <id>", "Remove one waiver from either scope", cobra.ExactArgs(1))
	cmd.Flags().Int("pr", 0, "look for the waiver in this pull request's scope too (§7.4.4)")
	return cmd
}
