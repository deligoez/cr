package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/sandbox"
	"github.com/deligoez/cr/internal/state"
)

// testRunResult is what `cr test` reports: what ran, where it ran, and what it
// exited with.
//
// The sandbox path is printed rather than assumed. §5.2.1 has the suite run
// inside the sandbox and nowhere else, and a reader told only that tests ran
// cannot tell a run at the pull request head from one against their own
// half-edited working tree.
//
// The runner's output is not here. It reaches the reader as it is produced,
// on standard error, because §12.1 keeps stdout for the command's own result;
// §5.2.1 also asks for the exit code, the duration and a bounded tail of that
// output to be recorded, and that record is §5.2.4's, which is a separate task.
type testRunResult struct {
	// Sandbox is the worktree the command ran in.
	Sandbox string `json:"sandbox"`
	// Command is the argv as it ran, program included, so what is
	// reported can be pasted back into a shell.
	Command []string `json:"command"`
	// Filter is the expression the run was narrowed to, absent when the
	// whole suite ran.
	Filter string `json:"filter,omitempty"`
	// ExitCode is the runner's own exit status. It is reported rather than
	// interpreted: a failing suite is an ordinary outcome, and §5.2.5 and
	// §5.3.4 are what read the number.
	ExitCode int `json:"exit_code"`
	// Honesty carries §5.1.6's recreation notice when the sandbox had to
	// be rebuilt before the run. It is a field on the payload rather than
	// a second stream, so the reader of the JSON document and the reader
	// of the terminal are told the same thing by the same values, and
	// §11.1 exempts it from `--quiet`.
	Honesty []string `json:"honesty"`
}

// Text names the sandbox and the command before the outcome, because the first
// two are what make the third mean anything.
func (r *testRunResult) Text(w *writer) string {
	var out strings.Builder
	fmt.Fprintf(&out, "ran in %s\n", w.accent(r.Sandbox))
	fmt.Fprintf(&out, "  command %s\n", strings.Join(r.Command, " "))
	fmt.Fprintf(&out, "  filter  %s\n", listedOrNone(r.Filter))
	fmt.Fprintf(&out, "  exit    %d", r.ExitCode)
	for _, disclosed := range r.Honesty {
		fmt.Fprintf(&out, "\n%s", disclosed)
	}
	return out.String()
}

// listedOrNone renders an optional single value the way listed renders an
// optional list, so an absent filter reads as an answer rather than as a blank.
func listedOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

// newTestCmd runs the profile's test command inside the sandbox (§11, §5.2.1).
//
// The sandbox is checked before the run rather than trusted, per §5.1.6, and a
// sandbox that fails the check is rebuilt and the rebuild reported. That order
// is the contract: a suite run in a sandbox still holding an unreverted mutation
// measures the mutation, and reports the result under the head's name.
func newTestCmd(out *writer) *cobra.Command {
	var filter string
	cmd := &cobra.Command{
		Use:   "test " + prPlaceholder,
		Short: "Run the profile's test command inside the sandbox",
		Args:  prArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pr, err := parsePR(args[0])
			if err != nil {
				return err
			}
			owner, repo, err := repoOf(cmd)
			if err != nil {
				return err
			}
			layout, err := state.Default()
			if err != nil {
				return err
			}
			round, err := layout.Briefed(owner, repo, pr)
			if err != nil {
				return err
			}
			dir, err := repoDir()
			if err != nil {
				return err
			}
			// The profile the round resolved, for the reason
			// `cr sandbox create` reads the same field: §3.7 makes
			// `cr brief` its one writer, and a command that
			// re-selected one could run a suite the roles of this
			// round were never briefed against.
			resolved, file, err := roundProfile(layout, round.ProfileID)
			if err != nil {
				return err
			}
			argv, err := resolved.TestArgv(file, filter)
			if err != nil {
				return err
			}
			ready, err := sandbox.Ensure(&sandbox.Sources{
				Layout:      layout,
				Owner:       owner,
				Repo:        repo,
				PR:          pr,
				Head:        round.Head,
				RepoDir:     dir,
				Copy:        resolved.Sandbox.Copy,
				Setup:       resolved.Sandbox.Setup,
				ProfileFile: file,
			}, resolved.LeftoverGlob())
			if err != nil {
				return err
			}
			// Standard error, because §12.1 keeps stdout for the
			// document below and a suite that prints nothing until
			// it finishes is a suite nobody can tell from a hang.
			code, err := sandbox.Run(argv, ready.Path, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			return out.emit(&testRunResult{
				Sandbox:  ready.Path,
				Command:  argv,
				Filter:   filter,
				ExitCode: code,
				Honesty:  recreationNotice(ready),
			})
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "",
		"narrow the run to a subset, passed as the profile's tests.filter_flag")
	return cmd
}

// recreationNotice renders §5.1.6's disclosure, and an empty report when the
// sandbox passed the check as it stood.
//
// The wording is asked of the notice rather than built here, as `cr brief` asks
// its disclosures for theirs: a sentence composed at the call site is a second
// answer that can disagree with the data printed beside it.
func recreationNotice(ready *sandbox.Ready) []string {
	if ready.Recreated == nil {
		return []string{}
	}
	return []string{ready.Recreated.Disclosure()}
}
