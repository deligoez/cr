package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/profile"
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
	// Paths are the `--path` values the run was narrowed to, in the
	// order they were given, and absent when none was. They are reported
	// beside the filter because §5.2.2 keys a baseline by both: a caller
	// about to run a probe needs to know which population this run id
	// stands for.
	Paths []string `json:"paths,omitempty"`
	// ExitCode is the runner's own exit status. It is reported rather than
	// interpreted: a failing suite is an ordinary outcome, and §5.2.5 and
	// §5.3.4 are what read the number.
	ExitCode int `json:"exit_code"`
	// TimedOut says the run was killed for exceeding
	// `tests.timeout_seconds` (§5.2.3).
	//
	// It is reported beside the exit code rather than left to the run
	// record, because the exit code alone cannot carry it: a process cr
	// killed reports what the platform reports for a killed process, which
	// is the same shape of number a runner that decided to fail produces.
	// A reader told only `exit -1` would have to guess which happened, and
	// §5.3.4 puts the two on different rungs.
	TimedOut bool `json:"timed_out"`
	// TestsRun and TestsFailed are the counts §5.2.4 stored, absent when
	// the profile's patterns could not derive them, and Passed is §5.2.5's
	// verdict on the run.
	//
	// They are printed because this command is where an operator checks
	// what a profile's count mode reads from their own suite. Measured
	// 2026-09-22 against deligoez/cr-qa-go: v0.6.0 printed neither, and
	// the counts reached only runs.ndjson under the state root, so the
	// occurrence mode v0.6.0 shipped could not be checked from the command
	// that runs it.
	TestsRun    *int `json:"tests_run,omitempty"`
	TestsFailed *int `json:"tests_failed,omitempty"`
	Passed      bool `json:"passed"`
	// OutputTail is the `output_tail` §5.2.4 stored for the run, so the
	// output the counts were read from is in the command's own document
	// and not only in runs.ndjson; `--compact` omits it (§12.5).
	OutputTail string `json:"output_tail"`
	// Contaminated is why §5.1.6's check failed after the run, and empty
	// when it passed. A run it names measured a sandbox that had drifted
	// from the head under it, so §5.2.5's verdict on it is `false`
	// whatever the counts said and no probe can be graded against it.
	//
	// It is an ordinary field rather than an entry in Honesty because
	// §11.1's list of what survives `--quiet` is closed and this is not on
	// it. §5.1.6's recreation notice is, and a reader running with
	// `--quiet` is told about this run on the next one, when the sandbox
	// this contamination condemns is rebuilt.
	Contaminated string `json:"contaminated,omitempty"`
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
	fmt.Fprintf(&out, "  paths   %s\n", listed(r.Paths))
	fmt.Fprintf(&out, "  exit    %d\n", r.ExitCode)
	fmt.Fprintf(&out, "  counts  %s\n", countsText(r.TestsRun, r.TestsFailed))
	fmt.Fprintf(&out, "  passed  %t\n", r.Passed)
	if r.TimedOut {
		// §5.2.3's outcome, said in words, because the exit code
		// beside it is the platform's number for a killed process and
		// reads as an ordinary failure.
		fmt.Fprintf(&out, "  %s\n", w.accent("killed for exceeding tests.timeout_seconds"))
	}
	if r.Contaminated != "" {
		// Said before the run id, because it is what the run id
		// means: a record nothing may be graded against.
		fmt.Fprintf(&out, "  %s\n", w.accent("contaminated, per §5.1.6: "+r.Contaminated))
	}
	fmt.Fprintf(&out, "  run     %s", r.Run)
	for _, warned := range r.Warnings {
		fmt.Fprintf(&out, "\n%s", warned)
	}
	out.WriteString(w.disclose("\n", "", r.Honesty...))
	return out.String()
}

// countsText renders the run's counts, and says so when the profile's patterns
// could not derive them rather than printing a zero nothing measured.
func countsText(executed, failed *int) string {
	if executed == nil || failed == nil {
		return "not derived"
	}
	return fmt.Sprintf("%d ran, %d failed", *executed, *failed)
}

// listedOrNone renders an optional single value the way listed renders an
// optional list, so an absent filter reads as an answer rather than as a blank.
func listedOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

// testTarget is what one `cr test` invocation resolved before anything ran:
// where the run happens, what it runs, and the round it is recorded against.
//
// It is a type rather than eight return values because every field of it is
// read by the run that follows, and because holding the resolution apart is
// what keeps newTestCmd inside the complexity the gate allows.
type testTarget struct {
	layout state.Layout
	owner  string
	repo   string
	pr     int
	// round is the round `cr brief` opened, whose head the sandbox is
	// checked out at and whose stamp every record takes.
	round state.Round
	// resolved and file are the round's profile and the file it came out
	// of, so a refusal can name what to open.
	resolved profile.Profile
	file     string
	// argv is the runner as §5.2.1 narrows it, filter and paths included.
	argv []string
	// src is §5.1's sandbox, as §5.1.6's check and §5.1.8's refusal read it.
	src *sandbox.Sources
}

// resolveTestTarget reads the round, its profile and the argv §5.2.1 runs, in
// the order the sections put them: the command line first, then the round,
// then the profile the round recorded.
//
// The profile is the round's resolved one, for the reason `cr sandbox create`
// reads the same field: §3.7 makes `cr brief` its one writer, and a command
// that re-selected one could run a suite the roles of this round were never
// briefed against.
func resolveTestTarget(
	cmd *cobra.Command, args []string, filter string, paths []string,
) (*testTarget, error) {
	pr, err := parsePR(args[0])
	if err != nil {
		return nil, err
	}
	// §5.2.1's three conditions on a `--path`, asked before any state is
	// read: a malformed invocation is §11.2's 2, and a run refused for its
	// command line must have built no sandbox and executed nothing.
	if err := run.CheckPaths(paths); err != nil {
		return nil, err
	}
	owner, repo, err := repoOf(cmd)
	if err != nil {
		return nil, err
	}
	layout, err := state.Default()
	if err != nil {
		return nil, err
	}
	round, err := briefedRound(layout, owner, repo, pr)
	if err != nil {
		return nil, err
	}
	// §9.3.2: this command writes the round's run record and builds the
	// sandbox at the round's head when §5.1.6 finds none, so a head that
	// moved under the round refuses before anything runs.
	if err := round.RefuseStale(); err != nil {
		return nil, err
	}
	dir, err := repoDir()
	if err != nil {
		return nil, err
	}
	resolved, file, err := roundProfile(layout, round.ProfileID)
	if err != nil {
		return nil, err
	}
	argv, err := resolved.TestArgv(file, filter, paths)
	if err != nil {
		return nil, err
	}
	return &testTarget{
		layout: layout, owner: owner, repo: repo, pr: pr,
		round: round, resolved: resolved, file: file, argv: argv,
		src: &sandbox.Sources{
			Layout:      layout,
			Owner:       owner,
			Repo:        repo,
			PR:          pr,
			Head:        round.Head,
			RepoDir:     dir,
			Copy:        resolved.Sandbox.Copy,
			Setup:       resolved.Sandbox.Setup,
			Require:     resolved.Sandbox.Require,
			ProfileFile: file,
		},
	}, nil
}

// newTestCmd runs the profile's test command inside the sandbox (§11, §5.2.1).
//
// The sandbox is checked before the run rather than trusted, per §5.1.6, and a
// sandbox that fails the check is rebuilt and the rebuild reported. That order
// is the contract: a suite run in a sandbox still holding an unreverted mutation
// measures the mutation, and reports the result under the head's name.
//
//nolint:funlen // measured 2026-09-16 at 82 lines against a limit of 60; refactor to clear, never raise the limit
func newTestCmd(out *writer) *cobra.Command {
	var filter string
	var paths []string
	cmd := &cobra.Command{
		Use:   "test " + prPlaceholder,
		Short: "Run the profile's test command inside the sandbox",
		Args:  prArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := resolveTestTarget(cmd, args, filter, paths)
			if err != nil {
				return err
			}
			layout, owner, repo, pr := target.layout, target.owner, target.repo, target.pr
			round, resolved, src := target.round, target.resolved, target.src
			argv := target.argv
			ready, uncopied, err := ensureAnnounced(cmd, out, src, resolved.LeftoverGlob(), argv)
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
				resolved.Tests.CountPattern, resolved.Tests.FailedPattern, resolved.CountsOccurrences())
			if err != nil {
				return err
			}
			// The echo to standard error is the informational half
			// and `--quiet` takes it (§11.1); the tail and the
			// counter are the stored half and see the stream either way.
			log := io.MultiWriter(out.informational(cmd.ErrOrStderr()), tail, counter)
			// §5.6.1's lock, taken around the run itself and
			// nothing else. What it protects is the resource the
			// suite touches — the test database the profile
			// configures — so it is held for exactly as long as
			// something is running against it.
			probe, err := lockProbe(layout, owner, repo, src.RepoDir, round.ProfileID)
			if err != nil {
				return err
			}
			// §5.2.3's budget, which §2.4 defaults to 900 seconds
			// and refuses to leave unset, so the run is always
			// bounded by a number the profile chose.
			budget := time.Duration(resolved.Tests.TimeoutSeconds) * time.Second
			// The runner lock the runner inherits, so a later run
			// finds this runner if cr does not outlive it (§5.3.3).
			runner, err := layout.LockRunner(owner, repo, pr)
			if err != nil {
				return errors.Join(err, probe.Unlock())
			}
			started := time.Now()
			code, timedOut, err := sandbox.Run(argv, ready.Path, log, budget, runner)
			took := time.Since(started)
			// Joined rather than branched, as every other release
			// in cr is: the lock goes whether or not the run did.
			lingering, released := runner.Release()
			if err := errors.Join(err, released, probe.Unlock()); err != nil {
				return err
			}
			// §5.1.6's check again, now that the suite has finished
			// with the sandbox. Round 12's baseline-contamination
			// finding is what puts it here: §5.1.7 voids a probe
			// whose post-run check fails, and a `cr test` run that
			// dirtied a tracked file under itself was voided by
			// nothing at all — so it could be stored `passed: true`
			// and later resolved as the baseline a probe is graded
			// against. The check is the same check; what differs is
			// only that this one runs after.
			contaminated, err := sandbox.Unclean(src, resolved.LeftoverGlob())
			if err != nil {
				return err
			}
			executed, failed := counter.Counts(code)
			stamp := state.Stamp{Head: round.Head, Round: round.Round}
			stored := &run.Record{
				Filter:       filter,
				Paths:        paths,
				ExitCode:     code,
				TimedOut:     timedOut,
				DurationMS:   took.Milliseconds(),
				TestsRun:     executed,
				TestsFailed:  failed,
				OutputTail:   tail.String(),
				Contaminated: contaminated != "",
				Sandbox:      ready.Generation,
			}
			recorded, err := recordRun(layout, owner, repo, pr, stamp, stored)
			if err != nil {
				return err
			}
			return out.emit(&testRunResult{
				Run:          recorded,
				Sandbox:      ready.Path,
				Command:      argv,
				Filter:       filter,
				Paths:        paths,
				ExitCode:     code,
				TimedOut:     timedOut,
				TestsRun:     stored.TestsRun,
				TestsFailed:  stored.TestsFailed,
				Passed:       stored.Passed,
				OutputTail:   stored.OutputTail,
				Contaminated: contaminated,
				Warnings:     []string{probe.CollisionWarning()},
				Honesty: append(append(append(append(recreationNotice(ready), survivorNotice(lingering)...),
					uncopied...), resolved.StaleDisclosures()...),
					stored.Uncounted(resolved.Tests.CountPattern)...),
			})
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "",
		"narrow the run to a subset, passed as the profile's tests.filter_flag")
	cmd.Flags().StringArrayVar(&paths, "path", nil,
		"narrow the run to a path inside the sandbox, passed through the profile's "+
			"tests.paths_arg; repeatable")
	return cmd
}

// recreationNotice renders §5.1.6's disclosure, and §5.3.3's for a runner an
// earlier cr left alive and this run killed, and an empty report when the
// sandbox passed the check as it stood with no such runner.
//
// The wording is asked of the notice rather than built here, as `cr brief` asks
// its disclosures for theirs: a sentence composed at the call site is a second
// answer that can disagree with the data printed beside it.
func recreationNotice(ready *sandbox.Ready) []string {
	notices := make([]string, 0, 2)
	if ready.Stopped != nil {
		notices = append(notices, ready.Stopped.Disclosure())
	}
	if ready.Recreated != nil {
		notices = append(notices, ready.Recreated.Disclosure())
	}
	return notices
}

// survivorNotice renders §5.6.3's disclosure for a process of this run's test
// runner that still held the runner lock when the run ended, and nothing when
// the lock came free.
//
// It is a slice rather than a string so that a run with nothing to disclose
// adds nothing, the shape recreationNotice already uses.
func survivorNotice(lingering bool) []string {
	if !lingering {
		return nil
	}
	return []string{sandbox.RunnerSurvivor{}.Disclosure()}
}

// lockProbe takes §5.6.1's advisory lock for the repository under review and
// the round's profile, waiting up to `probe.lock_timeout_seconds`.
//
// repoDir is the directory cr was run in, and the lock is named after the root
// of the repository it lies in, which git.Toplevel answers. §5.6.1 names the
// lock after the absolute path of the repository under review, and the
// directory itself would name a different lock for every directory the command
// happened to be invoked from — audit round 5 took a second lock for
// `<root>/internal` while the root's was held — which is the same repository
// sharing a test database with itself under two names.
//
// The profile id comes from meta.json, for the reason the test command and
// `cr sandbox create` both read it there: §3.7 makes `cr brief` its one writer,
// and a run that re-selected a profile could take a different lock than the run
// it is racing.
func lockProbe(l state.Layout, owner, repo, repoDir, profileID string) (*state.ProbeLock, error) {
	root, err := git.Toplevel(repoDir)
	if err != nil {
		return nil, err
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
	return l.LockProbe(root, profileID, wait)
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
	// §5.2.5's verdict, computed here rather than by the caller for the
	// reason the id is: both are cr's to derive, and a `passed` a caller
	// could set is one a caller could set wrongly. §5.3.5 and §5.4.4 let a
	// probe support a `probed` grade only when its baseline passed, so
	// this field is what stands between a measured run and a graded
	// assertion.
	record.Passed = record.Verdict()
	if err := state.AppendStamped(held, state.FileRuns, at, []*run.Record{record}); err != nil {
		return "", err
	}
	return record.ID, nil
}
