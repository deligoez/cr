package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/run"
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
// on standard error, because §12.1 keeps stdout for the command's own result,
// and the last `tests.output_tail_bytes` of it are stored in the run record
// §5.2.4 requires of every run.
type testRunResult struct {
	// Run is the id of the run record §5.2.4 stored for this run. It is
	// printed because it is the id space a probe's `baseline` field
	// references (§5.5), so a caller about to run a probe needs it and
	// would otherwise have to parse runs.ndjson to find it.
	Run string `json:"run"`
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
	// Warnings carries §5.6.3's collision warning: the probe lock covers
	// cr's own runs and can cover nothing else.
	//
	// It is a field of its own rather than another entry in Honesty,
	// because §11.1's list of what survives `--quiet` is closed and this
	// is not on it. Folding the two together would quietly widen that
	// exemption the moment the suppression is implemented.
	Warnings []string `json:"warnings"`
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
	fmt.Fprintf(&out, "  exit    %d\n", r.ExitCode)
	fmt.Fprintf(&out, "  run     %s", r.Run)
	for _, warned := range r.Warnings {
		fmt.Fprintf(&out, "\n%s", warned)
	}
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
			// The tail is teed off the same stream rather than read
			// back afterwards, so what §5.2.4 stores is what the
			// person watching the command saw.
			tail := run.NewTail(resolved.Tests.OutputTailBytes)
			// §5.2.1's two counts are read off that same merged
			// stream, so what they are matched against is what was
			// shown and what the tail ends with — and off the whole
			// of it rather than off the tail, because the counts are
			// summed over every match and a truncated view loses
			// matches silently.
			counter, err := run.NewCounter(
				resolved.Tests.CountPattern, resolved.Tests.FailedPattern)
			if err != nil {
				return err
			}
			log := io.MultiWriter(cmd.ErrOrStderr(), tail, counter)
			// §5.6.1's lock, taken around the run itself and
			// nothing else. What it protects is the resource the
			// suite touches — the test database the profile
			// configures — so it is held for exactly as long as
			// something is running against it.
			probe, err := lockProbe(layout, owner, repo, dir, round.ProfileID)
			if err != nil {
				return err
			}
			started := time.Now()
			code, err := sandbox.Run(argv, ready.Path, log)
			took := time.Since(started)
			// Joined rather than branched, as every other release
			// in cr is: the lock goes whether or not the run did.
			if err := errors.Join(err, probe.Unlock()); err != nil {
				return err
			}
			executed, failed := counter.Counts()
			stamp := state.Stamp{Head: round.Head, Round: round.Round}
			recorded, err := recordRun(layout, owner, repo, pr, stamp, &run.Record{
				Filter:      filter,
				ExitCode:    code,
				DurationMS:  took.Milliseconds(),
				TestsRun:    executed,
				TestsFailed: failed,
				OutputTail:  tail.String(),
			})
			if err != nil {
				return err
			}
			return out.emit(&testRunResult{
				Run:      recorded,
				Sandbox:  ready.Path,
				Command:  argv,
				Filter:   filter,
				ExitCode: code,
				Warnings: []string{probe.CollisionWarning()},
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

// lockProbe takes §5.6.1's advisory lock for the repository under review and
// the round's profile, waiting up to `probe.lock_timeout_seconds`.
//
// repoDir is the checkout cr was run in, resolved to an absolute path. §5.6.1
// names the lock after that path, and a relative one would name a different
// lock for every directory the command happened to be invoked from — which is
// the same repository sharing a test database with itself under two names.
//
// The profile id comes from meta.json, for the reason the test command and
// `cr sandbox create` both read it there: §3.7 makes `cr brief` its one writer,
// and a run that re-selected a profile could take a different lock than the run
// it is racing.
func lockProbe(l state.Layout, owner, repo, repoDir, profileID string) (*state.ProbeLock, error) {
	absolute, err := filepath.Abs(repoDir)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve %s: %w", repoDir, err)
	}
	resolved, err := config.Resolve(config.Sources{
		Environ:      os.Environ(),
		GlobalConfig: l.Config(),
		RepoConfig:   l.RepoConfig(owner, repo),
	})
	if err != nil {
		return nil, err
	}
	wait := time.Duration(resolved.Int("probe.lock_timeout_seconds")) * time.Second
	return l.LockProbe(absolute, profileID, wait)
}

// recordRun stores §5.2.4's run record and returns the id it was given.
//
// The id is allocated under the lock rather than before it. §5.5 has a probe
// reference its baseline by this id, so two runs that both read runs.ndjson
// before either wrote would allocate the same one and a stored probe would
// point at whichever of the two landed second. The read inside the lock is
// safe unlocked per §2.3.2, and no other writer can be between it and the
// append.
//
// head and round are not set here and could not be: state.AppendStamped writes
// both on the way out (§2.3.3), from the round meta.json recorded.
func recordRun(
	layout state.Layout, owner, repo string, pr int, at state.Stamp, record *run.Record,
) (string, error) {
	held, err := layout.LockPR(owner, repo, pr)
	if err != nil {
		return "", err
	}
	id, err := appendRun(layout, owner, repo, pr, held, at, record)
	if err != nil {
		// The lock is released on the way out of every branch, and
		// the write's own failure is what the caller is told about.
		_ = held.Unlock()
		return "", err
	}
	if err := held.Unlock(); err != nil {
		return "", err
	}
	return id, nil
}

// appendRun is recordRun's work, held apart so the lock is released on every
// path out of it.
func appendRun(
	layout state.Layout, owner, repo string, pr int,
	held *state.Lock, at state.Stamp, record *run.Record,
) (string, error) {
	stored, err := state.ReadRecords[run.Record](layout, owner, repo, pr, state.FileRuns)
	if err != nil {
		return "", err
	}
	record.ID = run.NextID(stored)
	if err := state.AppendStamped(held, state.FileRuns, at, []*run.Record{record}); err != nil {
		return "", err
	}
	return record.ID, nil
}
