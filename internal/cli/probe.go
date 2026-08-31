package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/run"
	"github.com/deligoez/cr/internal/sandbox"
	"github.com/deligoez/cr/internal/state"
)

// probeRunResult is what `cr probe run` reports: which experiment was
// performed, where, and what came of it.
//
// The baseline is printed beside the result because the two are read together.
// §5.3.5 lets only `no-test-failed` prove a gap, and only when the baseline run
// passed — so a result without the run it was measured against is a number a
// reader cannot act on.
type probeRunResult struct {
	// Probe is the id of the probe record §5.5 stored for this run.
	Probe string `json:"probe"`
	// Kind is the sort of probe that ran (§5.5).
	Kind string `json:"kind"`
	// Sandbox is the worktree the mutation was applied in.
	Sandbox string `json:"sandbox"`
	// Command is the argv the runner was started with, program included.
	Command []string `json:"command"`
	// Filter is the expression the run was narrowed to, absent when the
	// whole suite ran. §5.3.6 has a filtered run prove the gap only for
	// the tests it selected, so it is reported rather than implied.
	Filter string `json:"filter,omitempty"`
	// Result is §5.5's `result`, after §5.1.7 has had its say.
	Result string `json:"result"`
	// Baseline is the id of the run record §5.2.6 resolved.
	Baseline string `json:"baseline"`
	// Run is the id of the run record for the mutated run, and empty when
	// §5.3.4's first rung fired: a patch that did not apply runs no tests.
	Run string `json:"run,omitempty"`
	// Voided is why §5.1.6's post-run check failed, and empty when it
	// passed. A probe it names establishes nothing in either direction
	// (§5.1.7), which is why the ladder's own answer is not what Result
	// carries.
	Voided string `json:"voided,omitempty"`
	// Warnings carries §5.6.3's collision warning, as `cr test` does.
	Warnings []string `json:"warnings"`
	// Honesty carries §5.1.6's recreation notice, which §11.1 exempts
	// from `--quiet`.
	Honesty []string `json:"honesty"`
}

// Text names the experiment before its answer, because the answer means
// nothing without the patch, the filter, and the baseline that framed it.
func (r *probeRunResult) Text(w *writer) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s probe in %s\n", r.Kind, w.accent(r.Sandbox))
	fmt.Fprintf(&out, "  command  %s\n", strings.Join(r.Command, " "))
	fmt.Fprintf(&out, "  filter   %s\n", listedOrNone(r.Filter))
	fmt.Fprintf(&out, "  baseline %s\n", r.Baseline)
	if r.Voided != "" {
		// Said before the result, because it is what the result
		// means: a value the ladder did not produce.
		fmt.Fprintf(&out, "  %s\n", w.accent("voided, per §5.1.7: "+r.Voided))
	}
	fmt.Fprintf(&out, "  result   %s\n", w.accent(r.Result))
	fmt.Fprintf(&out, "  probe    %s", r.Probe)
	for _, warned := range r.Warnings {
		fmt.Fprintf(&out, "\n%s", warned)
	}
	for _, disclosed := range r.Honesty {
		fmt.Fprintf(&out, "\n%s", disclosed)
	}
	return out.String()
}

// suite is everything one run of the profile's test command inside the sandbox
// needs, resolved once for the several runs a probe performs.
//
// §5.2.6 has a probe perform its missing baselines before it runs, so a single
// `cr probe run` starts the runner more than once — and every one of those runs
// has to be the same run but for its filter, or the comparison the probe rests
// on is a comparison of two different things.
type suite struct {
	profile profile.Profile
	// file is the profile file the fields came from, so a refusal can
	// name what to open.
	file string
	// path is the sandbox §5.1.6 admitted, and the only directory the
	// runner is ever started in.
	path string
	// log is where the runner's output reaches the reader as it is
	// produced, which §12.1 keeps off stdout.
	log io.Writer
}

// measuredRun is one execution of the suite, before §5.1.6's post-run check and
// §5.3.4's ladder read it.
type measuredRun struct {
	// record is §5.2.4's run record, less the fields the writer owns and
	// less `contaminated`, which the caller sets from a check it has to
	// run at a moment only it knows.
	record *run.Record
	// unstarted says the runner could not be started at all, which
	// §5.3.4's third rung answers rather than the command failing: a
	// probe whose runner is missing has a result, and it is `error`.
	unstarted bool
}

// perform runs the suite once, narrowed to filter.
//
// The cleanliness check is deliberately not here. §5.1.7 has the post-run check
// decide what the record says, and for a mutation probe that check has to run
// after the revert — before it, every probe would find its own mutation and
// void itself. Where the check belongs is therefore the caller's to know, and
// leaving it out is what keeps this one function honest for both runs.
func (s *suite) perform(filter string) (*measuredRun, error) {
	argv, err := s.profile.TestArgv(s.file, filter)
	if err != nil {
		return nil, err
	}
	tail := run.NewTail(s.profile.Tests.OutputTailBytes)
	counter, err := run.NewCounter(s.profile.Tests.CountPattern, s.profile.Tests.FailedPattern)
	if err != nil {
		return nil, err
	}
	log := io.MultiWriter(s.log, tail, counter)
	budget := time.Duration(s.profile.Tests.TimeoutSeconds) * time.Second

	started := time.Now()
	code, timedOut, err := sandbox.Run(argv, s.path, log, budget)
	took := time.Since(started)

	unstarted := false
	if err != nil {
		var unrunnable *sandbox.RunError
		if !errors.As(err, &unrunnable) {
			return nil, err
		}
		// §5.3.4's third rung: a runner that could not be started is a
		// probe result rather than a command failure. `cr test` exits
		// 3 on the same error, and rightly — it was asked to run a
		// suite and could not — while a probe was asked what the suite
		// says about a mutation, and "nothing, it would not start" is
		// an answer §5.3.5 already refuses to grade on.
		unstarted = true
	}
	executed, failed := counter.Counts()
	return &measuredRun{
		record: &run.Record{
			Filter:      filter,
			ExitCode:    code,
			TimedOut:    timedOut,
			DurationMS:  took.Milliseconds(),
			TestsRun:    executed,
			TestsFailed: failed,
			OutputTail:  tail.String(),
		},
		unstarted: unstarted,
	}, nil
}

// newProbeCmd groups the probe commands of §11. It runs nothing itself, so an
// invocation naming no subcommand prints the help rather than performing an
// experiment nobody asked for.
func newProbeCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "probe",
		Short: "Execute and record a probe",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newProbeRunCmd(out))
	return cmd
}

// newProbeRunCmd applies a mutation, runs the tests, reverts the mutation, and
// records the result (§11, §5.3.2).
//
// The order is §5.3.2's own sentence, and the revert is not the last step of it
// but the frame around the middle two: state.UnderSandboxMutation writes the
// mutation, calls the run, and restores the files whether the run returned,
// failed, timed out, or panicked. §5.3.3 admits no path where the mutation
// stays on disk that cr is still alive to prevent, and a revert written after
// the run would be one `return err` away from being skipped.
func newProbeRunCmd(out *writer) *cobra.Command {
	var kind, patchFile, filter string
	cmd := &cobra.Command{
		Use:   "run " + prPlaceholder,
		Short: "Execute and record a probe",
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
			if kind != string(probe.Mutation) {
				return fmt.Errorf(
					"--kind %q: cr runs §5.3's mutation probe; §5.4's gap probe is not implemented yet",
					kind)
			}
			if patchFile == "" {
				return errors.New(
					"--patch is required: §5.3.1 has the agent supply the mutation as a unified diff")
			}
			body, err := os.ReadFile(patchFile)
			if err != nil {
				return err
			}
			files, err := git.ParsePatch(string(body))
			if err != nil {
				return err
			}
			if len(files) == 0 {
				return fmt.Errorf(
					"%s holds no hunk: §5.3.1's mutation is a unified diff against a sandbox file",
					patchFile)
			}
			return runMutationProbe(cmd, out, &probeRequest{
				owner: owner, repo: repo, pr: pr,
				patch: string(body), files: files, filter: filter,
			})
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "",
		"the probe to run: mutation (§5.3)")
	cmd.Flags().StringVar(&patchFile, "patch", "",
		"the unified diff to apply, per §5.3.1")
	cmd.Flags().StringVar(&filter, "filter", "",
		"narrow the run to a subset, passed as the profile's tests.filter_flag")
	return cmd
}

// probeRequest is one `cr probe run` invocation, as the command line gave it.
type probeRequest struct {
	owner string
	repo  string
	pr    int
	// patch is the diff as it was handed over, which §5.5's `input` row
	// stores whole: the record has to say which experiment was performed.
	patch string
	// files is the same diff, parsed for application.
	files []git.PatchedFile
	// filter is what the run is narrowed to, empty for the whole suite.
	filter string
}

// runMutationProbe performs §5.3.2's cycle and records what it produced.
func runMutationProbe(cmd *cobra.Command, out *writer, request *probeRequest) error {
	layout, err := state.Default()
	if err != nil {
		return err
	}
	round, err := layout.Briefed(request.owner, request.repo, request.pr)
	if err != nil {
		return err
	}
	dir, err := repoDir()
	if err != nil {
		return err
	}
	// The profile the round resolved, for the reason `cr test` reads the
	// same field: §3.7 makes `cr brief` its one writer, and a probe that
	// re-selected one could measure a suite this round was never briefed
	// against.
	resolved, file, err := roundProfile(layout, round.ProfileID)
	if err != nil {
		return err
	}
	src := &sandbox.Sources{
		Layout:      layout,
		Owner:       request.owner,
		Repo:        request.repo,
		PR:          request.pr,
		Head:        round.Head,
		RepoDir:     dir,
		Copy:        resolved.Sandbox.Copy,
		Setup:       resolved.Sandbox.Setup,
		ProfileFile: file,
	}
	glob := resolved.LeftoverGlob()
	ready, err := sandbox.Ensure(src, glob)
	if err != nil {
		return err
	}
	tests := &suite{profile: resolved, file: file, path: ready.Path, log: cmd.ErrOrStderr()}
	stamp := state.Stamp{Head: round.Head, Round: round.Round}

	// §5.6.1's lock, taken once around every run this command performs.
	// The baselines of §5.2.6 run the same suite against the same test
	// database as the probe does, so a lock taken per run would either
	// leave the gaps between them open or, taken twice over, wait on
	// itself for the whole of `probe.lock_timeout_seconds`.
	locked, err := lockProbe(layout, request.owner, request.repo, dir, round.ProfileID)
	if err != nil {
		return err
	}
	performed, err := probeRuns(layout, tests, src, glob, request, &round, stamp)
	if err := errors.Join(err, locked.Unlock()); err != nil {
		return err
	}

	// §5.1.6's check, now that the suite has finished and the mutation is
	// off disk again. A check run before the revert would find every
	// probe's own mutation and void every probe.
	unclean, err := sandbox.Unclean(src, glob)
	if err != nil {
		return err
	}
	outcome := probe.Decide(probe.Ladder(performed.measured), unclean)

	record := &probe.Record{
		Kind:       probe.Mutation,
		Input:      request.patch,
		Filter:     request.filter,
		Result:     outcome.Result(),
		Baseline:   performed.baseline.ID(),
		DurationMS: performed.durationMS,
		OutputTail: performed.outputTail,
	}
	record.TestsRun, record.TestsFailed = performed.measured.TestsRun, performed.measured.TestsFailed
	if performed.mutated != nil {
		performed.mutated.Contaminated = unclean != ""
	}
	runID, probeID, err := recordProbe(
		layout, request.owner, request.repo, request.pr, stamp, performed.mutated, record)
	if err != nil {
		return err
	}
	// §5.1.7: a voided probe forces recreation before the next run. It
	// happens after the record lands, so the evidence the check found is
	// on disk before the sandbox it describes is rebuilt.
	if outcome.Voided() {
		if err := sandbox.ForceRecreation(src); err != nil {
			return err
		}
	}
	return out.emit(&probeRunResult{
		Probe:    probeID,
		Kind:     string(probe.Mutation),
		Sandbox:  ready.Path,
		Command:  performed.command,
		Filter:   request.filter,
		Result:   string(outcome.Result()),
		Baseline: performed.baseline.ID(),
		Run:      runID,
		Voided:   unclean,
		Warnings: []string{locked.CollisionWarning()},
		Honesty:  recreationNotice(ready),
	})
}

// performedProbe is everything the locked half of a probe produced.
type performedProbe struct {
	// baseline is the run §5.2.6 resolved, performing it first when the
	// head had none.
	baseline probe.Baseline
	// measured is what §5.3.4's ladder reads.
	measured probe.Measured
	// mutated is §5.2.4's record for the run on mutated code, and nil
	// when the patch did not apply and no tests were run.
	mutated *run.Record
	// command is the argv the mutated run was to be started with, which
	// is reported whether or not it ran.
	command []string
	// durationMS and outputTail are §5.5's rows for the probe's own run.
	durationMS int64
	outputTail string
}

// probeRuns performs §5.2.6's baselines and then the mutated run, all inside
// §5.6.1's lock.
//
// The baselines come first and never after, which is what keeps them un-probed:
// they run against the sandbox as §5.1 prepared it, so the record §5.2.6 stores
// is admissible by construction rather than by inspection.
func probeRuns(
	layout state.Layout, tests *suite, src *sandbox.Sources, glob string,
	request *probeRequest, round *state.Meta, stamp state.Stamp,
) (*performedProbe, error) {
	command, err := tests.profile.TestArgv(tests.file, request.filter)
	if err != nil {
		return nil, err
	}
	performed := &performedProbe{command: command}

	stored, err := state.ReadRecords[run.Record](
		layout, request.owner, request.repo, request.pr, state.FileRuns)
	if err != nil {
		return nil, err
	}
	performed.baseline, err = probe.Ensure(
		stored, round.Head, probe.Mutation, request.filter,
		func(spec probe.Spec) (run.Record, error) {
			return baselineRun(layout, tests, src, glob, request, stamp, spec)
		})
	if err != nil {
		return nil, err
	}

	mutations := make([]state.SandboxMutation, 0, len(request.files))
	for i := range request.files {
		file := &request.files[i]
		mutations = append(mutations, state.SandboxMutation{Path: file.Path, Apply: file.Apply})
	}
	var mutated *measuredRun
	err = layout.UnderSandboxMutation(
		request.owner, request.repo, request.pr, mutations, func() error {
			ran, err := tests.perform(request.filter)
			mutated = ran
			return err
		})
	var refused *git.ApplyError
	switch {
	case errors.As(err, &refused):
		// §5.3.4's first rung: the patch did not apply cleanly, so no
		// tests were run and the ladder answers `error`. It is a
		// result rather than a failure of the command — the agent
		// asked what the suite says about this mutation, and cr can
		// say that the mutation was never made.
		return performed, nil
	case err != nil:
		return nil, err
	}
	performed.mutated = mutated.record
	performed.durationMS = mutated.record.DurationMS
	performed.outputTail = mutated.record.OutputTail
	performed.measured = probe.Measured{
		Applied:     true,
		TimedOut:    mutated.record.TimedOut,
		Unstarted:   mutated.unstarted,
		ExitCode:    mutated.record.ExitCode,
		TestsRun:    mutated.record.TestsRun,
		TestsFailed: mutated.record.TestsFailed,
	}
	return performed, nil
}

// baselineRun performs one of §5.2.2's baselines and stores §5.2.4's record for
// it, which is what probe.Ensure hands back to §5.5's `baseline` column.
func baselineRun(
	layout state.Layout, tests *suite, src *sandbox.Sources, glob string,
	request *probeRequest, stamp state.Stamp, spec probe.Spec,
) (run.Record, error) {
	ran, err := tests.perform(spec.Filter)
	if err != nil {
		return run.Record{}, err
	}
	// §5.1.6's check after this run too, for round 12's
	// baseline-contamination reason: a baseline whose sandbox drifted
	// under it is nobody's baseline, and §5.2.5's verdict is what says so.
	unclean, err := sandbox.Unclean(src, glob)
	if err != nil {
		return run.Record{}, err
	}
	ran.record.Contaminated = unclean != ""
	if _, err := recordRun(
		layout, request.owner, request.repo, request.pr, stamp, ran.record); err != nil {
		return run.Record{}, err
	}
	return *ran.record, nil
}

// recordProbe stores §5.5's probe record and, when the mutation applied,
// §5.2.4's record for the run it was measured by.
//
// Both writes happen under one hold of §2.3.1's lock, because the run record
// carries the probe's id: allocating the id in one hold and referencing it in
// another would let a second cr run allocate the same one in between.
func recordProbe(
	layout state.Layout, owner, repo string, pr int, at state.Stamp,
	mutated *run.Record, record *probe.Record,
) (runID, probeID string, err error) {
	held, err := layout.LockPR(owner, repo, pr)
	if err != nil {
		return "", "", err
	}
	runID, probeID, err = appendProbe(layout, owner, repo, pr, held, at, mutated, record)
	if err != nil {
		// The lock is released on the way out of every branch, and
		// the write's own failure is what the caller is told about.
		_ = held.Unlock()
		return "", "", err
	}
	if err := held.Unlock(); err != nil {
		return "", "", err
	}
	return runID, probeID, nil
}

// appendProbe is recordProbe's work, held apart so the lock is released on
// every path out of it.
func appendProbe(
	layout state.Layout, owner, repo string, pr int,
	held *state.Lock, at state.Stamp, mutated *run.Record, record *probe.Record,
) (runID, probeID string, err error) {
	stored, err := state.ReadRecords[probe.Record](layout, owner, repo, pr, state.FileProbes)
	if err != nil {
		return "", "", err
	}
	record.ID = probe.NextID(stored)
	if mutated != nil {
		// §5.2.6's fence, set here and nowhere else: the probe's own
		// run carries the probe's id, so it can never be resolved as
		// the next probe's baseline.
		mutated.Probe = record.ID
		if runID, err = appendRun(layout, owner, repo, pr, held, at, mutated); err != nil {
			return "", "", err
		}
	}
	if err := state.AppendStamped(held, state.FileProbes, at, []*probe.Record{record}); err != nil {
		return "", "", err
	}
	return runID, record.ID, nil
}
