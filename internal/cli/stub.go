package cli

import (
	"github.com/spf13/cobra"
)

// notImplementedHint is what a caller who met a registered-but-unbuilt command
// should do next.
//
// It is one sentence for every stub rather than one per command, because the
// next step is the same in every case and a per-command hint would be a
// promise about which release implements what — a claim cr is in no position
// to make from inside the build that does not implement it.
const notImplementedHint = "the §11 command surface is complete before its behaviour is, " +
	"so this is a stub and not a broken install; check `cr --version` against the release notes"

// notImplementedError is the refusal of a §11 row whose surface is registered
// here and whose behaviour a later task owns.
//
// It is a type rather than a bare string so the refusal is distinguishable
// from a malformed invocation by inspection: a caller can tell "cr has never
// heard of this" from "cr knows this command and has not built it", which is
// exactly the distinction a complete surface exists to make. Nothing maps it
// in exit.go, so it takes exitCodeFor's fallback of §11.2's code 2 — §11.2
// enumerates five codes and has no row for a command that is registered and
// unbuilt, and inventing a sixth would renumber the contract invariant 5 pins.
type notImplementedError struct {
	// Command is the command as it was typed, e.g. `cr rules list`, taken
	// from cobra's own path so a stub reached through a group names the
	// group too.
	Command string
	// Hint names the next actionable step, per the convention that every
	// cr error carries one.
	Hint string
}

func (e *notImplementedError) Error() string {
	return e.Command + " is not implemented in this build: " + e.Hint
}

// notImplemented is the RunE of every stub. It reads the command's own path
// rather than a name repeated at the registration site, so a command that is
// later moved under a group cannot start naming itself wrongly.
func notImplemented(cmd *cobra.Command, _ []string) error {
	return &notImplementedError{Command: cmd.CommandPath(), Hint: notImplementedHint}
}

// stubAnnotation marks a command as registered ahead of its behaviour, and
// carries the hint its refusal will give.
//
// It exists so "is this command built" is readable off the tree rather than
// kept as a list somewhere. Several guards turn on that question — one asks
// which commands must refuse an unbriefed pull request with §11.2's code 4,
// another which may be run against a real repository and required to succeed —
// and every one of them had `Find` succeeding as its only proxy for it. A
// complete surface breaks that proxy, so the tree carries the answer instead:
// a command drops out of every such exemption on the commit that removes this
// mark, which is the commit that builds it.
const stubAnnotation = "cr.not_implemented"

// stubCmd registers one §11 row's argument shape without its behaviour.
//
// The argument shape is registered because it is the half of the contract a
// caller writes scripts against, and because §11.2's code 2 has to be reachable
// for a wrong command line even where the right one refuses: a stub that
// accepted anything would answer a mistyped invocation with the same message as
// a correct one.
func stubCmd(use, short string, args cobra.PositionalArgs) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        args,
		RunE:        notImplemented,
		Annotations: map[string]string{stubAnnotation: notImplementedHint},
	}
}

// groupCmd registers a §11 row that is a group of subcommands rather than a
// command of its own. It runs nothing itself, so an invocation naming no
// subcommand prints the help rather than doing something the user did not ask
// for — the shape `cr claims` and `cr sandbox` already take.
func groupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
}
