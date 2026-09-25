package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/config"
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
	// Proposal is the §5.7 proposal this run executed, absent for a run
	// the command line supplied.
	Proposal string `json:"proposal,omitempty"`
	// Regraded is §5.7.4's recomputation, absent when the run executed no
	// proposal or the proposal named no record.
	Regraded *Regraded `json:"regraded,omitempty"`
	// RerunOf is the probe §5.5.4's `--rerun` re-ran, absent for any other
	// run. The probe it names is left as it was: a re-run changes no
	// record's `probe` and no grade.
	RerunOf string `json:"rerun_of,omitempty"`
	// Kind is the sort of probe that ran (§5.5).
	Kind string `json:"kind"`
	// Sandbox is the worktree the experiment was performed in.
	Sandbox string `json:"sandbox"`
	// Command is the argv the runner was started with, program included.
	Command []string `json:"command"`
	// Filter is the expression the run was narrowed to, absent when the
	// whole suite ran. §5.3.6 has a filtered run prove the gap only for
	// the tests it selected, so it is reported rather than implied.
	Filter string `json:"filter,omitempty"`
	// Paths are the `--path` values, absent when none was given, and
	// reported for §5.3.6's reason too: a path-narrowed run proves the
	// gap only for the tests those paths hold. They are also the
	// population the baseline beside them measured.
	Paths []string `json:"paths,omitempty"`
	// Result is §5.5's `result`, after §5.1.7 has had its say.
	Result string `json:"result"`
	// Reason is the probe record's `reason`: why Result is `error` or
	// `inconclusive`, and absent for every other result.
	Reason string `json:"reason,omitempty"`
	// Establishes is what the sections governing this kind let the result
	// be read for: `gap` when a mutation probe proves one, `no-gap` when a
	// test caught the mutation, `nothing` when neither, and `undecided`
	// for a gap probe, whose support conditions §5.4.4 and §5.4.5 fix
	// against a finding that does not exist yet. It is reported rather
	// than left to be re-derived from the result, because the sections
	// point opposite ways — one lets a finding be graded `probed`, another
	// has the agent not raise the finding at all — and the difference is
	// not something a reader should have to reconstruct.
	Establishes string `json:"establishes"`
	// Target is the `path:line` the probe addresses: derived from the
	// patch for a mutation probe (§5.3.2), supplied with `--target` for a
	// gap probe (§5.5). It is reported rather than left to the record,
	// because it is what the agent would otherwise have to take on trust
	// before writing the finding §6.2.2 anchors there.
	Target string `json:"target"`
	// Baseline is the id of the run record §5.2.6 resolved.
	Baseline string `json:"baseline"`
	// Run is the id of the run record for the probe's own run, and empty
	// when §5.3.4's first rung fired: a patch that did not apply runs no
	// tests.
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
// nothing without the input, the filter, and the baseline that framed it.
func (r *probeRunResult) Text(w *writer) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s probe in %s\n", r.Kind, w.accent(r.Sandbox))
	fmt.Fprintf(&out, "  command  %s\n", strings.Join(r.Command, " "))
	fmt.Fprintf(&out, "  filter   %s\n", listedOrNone(r.Filter))
	fmt.Fprintf(&out, "  paths    %s\n", listed(r.Paths))
	fmt.Fprintf(&out, "  target   %s\n", r.Target)
	fmt.Fprintf(&out, "  baseline %s\n", r.Baseline)
	if r.Voided != "" {
		// Said before the result, because it is what the result
		// means: a value the ladder did not produce.
		fmt.Fprintf(&out, "  %s\n", w.accent("voided, per §5.1.7: "+r.Voided))
	}
	fmt.Fprintf(&out, "  result   %s\n", w.accent(r.Result))
	if r.Reason != "" {
		fmt.Fprintf(&out, "  reason   %s\n", r.Reason)
	}
	fmt.Fprintf(&out, "  evidence %s\n", establishedClause[r.Establishes])
	fmt.Fprintf(&out, "  probe    %s", r.Probe)
	for _, warned := range r.Warnings {
		fmt.Fprintf(&out, "\n%s", warned)
	}
	out.WriteString(w.disclose("\n", "", r.Honesty...))
	return out.String()
}

// What the sections governing a probe's kind let its outcome be read for, as
// the token probeRunResult.Establishes carries.
const (
	// establishesGap is §5.3.5's proof: `no-test-failed` against a
	// baseline that passed.
	establishesGap = "gap"
	// establishesNoGap is §5.3.7's disproof: a test caught the mutation.
	establishesNoGap = "no-gap"
	// establishesNothing is every result neither section lets a `probed`
	// grade rest on: §5.3.5's four, which §5.3.7 also reads no suppression
	// from, and the same four on §5.4.5's side.
	establishesNothing = "nothing"
	// establishesBehaviour is §5.4.5's own reading of a `passed` gap
	// probe. It is not `nothing`: the run did establish something, which
	// is that the behaviour the supplied test asserts is present — and it
	// is not `gap` either, because §5.4.5 is explicit that only §5.3's
	// `no-test-failed` establishes a missing test.
	establishesBehaviour = "behaviour"
	// establishesUndecided is a `failed` gap probe, and only that. §5.4.4
	// makes the support conditional on the finding's own `claim` field,
	// and no finding exists at the moment the experiment runs, so the
	// question cannot honestly be answered here in either direction.
	// §5.4.5 settles every other gap result at the run itself.
	establishesUndecided = "undecided"
)

// establishedBy reads §5.3.5 and §5.3.7 off the mutation probe that has just
// run.
//
// It asks the probe package rather than comparing result strings here, because
// the conditions are the section's and belong where the values are: Proves is
// handed the resolved baseline as well as the outcome, so the passing baseline
// §5.3.5 requires is one the runs actually recorded rather than one this
// command believed in.
func establishedBy(outcome probe.Outcome, baseline probe.Baseline) string {
	switch {
	case probe.Proves(outcome, baseline):
		return establishesGap
	case probe.Disproves(outcome):
		return establishesNoGap
	}
	return establishesNothing
}

// gapEstablishedBy reads §5.4.4 and §5.4.5 off the gap probe that has just run.
//
// Only `failed` is left open. §5.4.5 settles the other five at the run itself —
// four of them establish nothing a `probed` grade can rest on, and `passed`
// establishes a fact of its own that is not the one §5.3.5 licenses — while
// §5.4.4's conditions are read off a finding that does not exist yet.
//
// It asks the probe package rather than comparing result strings here, as
// establishedBy does: §5.5 fixes the vocabulary per kind and the values are
// spelled once, where the ladder that produces them is.
func gapEstablishedBy(record *probe.Record) string {
	switch {
	case probe.Reproduces(record):
		return establishesUndecided
	case probe.Present(record):
		return establishesBehaviour
	}
	return establishesNothing
}

// establishedClause says what each token means for the finding the agent is
// deciding whether to write, spelled out where the reader is rather than left
// to a section number they would have to open.
//
// The `gap` clause names the tests this run selected and not the suite, per
// §5.3.6: a filtered run proves the gap only for what the filter selected, and
// cr cannot establish that a filter selects the tests which would have caught
// the mutation, so it does not say so.
var establishedClause = map[string]string{
	establishesGap: "a test gap: the tests this run selected did not notice the mutation " +
		"and the baseline passed, so §5.3.5 lets a finding here be graded probed",
	establishesNoGap: "none: a test caught the mutation, so §5.3.7 has the finding not raised",
	establishesNothing: "none: neither §5.3.5 nor §5.4.5 lets this result support a probed " +
		"grade, so a record resting on it stays argued (§6.2) and is asked as a question (§6.3)",
	establishesBehaviour: "that the behaviour the supplied test asserts is present, and not " +
		"that the suite lacks a test — §5.4.5 leaves that to §5.3's no-test-failed; it supports " +
		"no probed grade, so a record resting on it stays argued (§6.2), is asked as a question " +
		"(§6.3), and carries severity at most medium",
	establishesUndecided: "not settled by the run: §5.4.4 lets a failed gap probe support a " +
		"finding only on a passing baseline and a claim mapped to the finding's unit, and the " +
		"claim field is the finding's own",
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
	// src names the pull request whose runner lock every run takes, so a
	// later run finds a runner cr did not outlive (§5.3.3).
	src *sandbox.Sources
	// generation names the sandbox at path, and is stamped on every run.
	generation string
	// lingering says some run of this probe — a baseline or the probe's
	// own — left a process still holding the runner lock (§5.6.3). It is
	// accumulated over every run rather than carried on one, because a
	// straggler of the baseline is running beside the probe's run, which is
	// the reading the disclosure exists for.
	lingering bool
}

// measuredRun is one execution of the suite, before §5.1.6's post-run check and
// §5.3.4's or §5.4.3's ladder read it.
type measuredRun struct {
	// record is §5.2.4's run record, less the fields the writer owns and
	// less `contaminated`, which the caller sets from a check it has to
	// run at a moment only it knows.
	record *run.Record
	// unstarted says the runner could not be started at all, which
	// §5.3.4's third rung and §5.4.3's second answer rather than the
	// command failing: a probe whose runner is missing has a result, and
	// it is `error`.
	unstarted bool
	// detail is what the attempt to start the runner reported, or the
	// signal a started runner exited on, and is empty when neither
	// happened. It is what the reason an `error` result carries names.
	detail string
}

// perform runs the suite once, narrowed to filter and to paths.
//
// Both narrowings travel together because §5.2.2 keys a baseline by the pair:
// the population a probe's own run measured and the population its baseline
// measured have to be the same population, and a run that took one of the two
// from the request and the other from somewhere else would be comparing two
// different suites.
//
// The cleanliness check is deliberately not here. §5.1.7 has the post-run check
// decide what the record says, and for a probe that check has to run after the
// mutation is reverted or the probe file removed — before it, every probe would
// find its own artefact and void itself. Where the check belongs is therefore
// the caller's to know, and leaving it out is what keeps this one function
// honest for every run.
func (s *suite) perform(filter string, paths []string) (*measuredRun, error) {
	argv, err := s.profile.TestArgv(s.file, filter, paths)
	if err != nil {
		return nil, err
	}
	tail := run.NewTail(s.profile.Tests.OutputTailBytes)
	counter, err := run.NewCounter(
		s.profile.Tests.CountPattern, s.profile.Tests.FailedPattern, s.profile.CountsOccurrences())
	if err != nil {
		return nil, err
	}
	log := io.MultiWriter(s.log, tail, counter)
	budget := time.Duration(s.profile.Tests.TimeoutSeconds) * time.Second

	runner, err := s.src.Layout.LockRunner(s.src.Owner, s.src.Repo, s.src.PR)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	exit, err := sandbox.RunExit(argv, s.path, log, budget, runner)
	took := time.Since(started)
	// A disclosure and never a result: what the suite said is unchanged by
	// a straggler, so it is accumulated and reported beside the probe
	// rather than allowed to suppress or re-grade it.
	lingering, released := runner.Release()
	s.lingering = s.lingering || lingering
	if released != nil {
		return nil, errors.Join(err, released)
	}

	unstarted, detail := false, exit.Signal
	if err != nil {
		var unrunnable *sandbox.RunError
		if !errors.As(err, &unrunnable) {
			return nil, err
		}
		// §5.3.4's third rung and §5.4.3's second: a runner that
		// could not be started is a probe result rather than a
		// command failure. `cr test` exits 3 on the same error, and
		// rightly — it was asked to run a suite and could not — while
		// a probe was asked what the suite says, and "nothing, it
		// would not start" is an answer §5.3.5 and §5.4.5 already
		// refuse to grade on.
		unstarted, detail = true, unrunnable.Error()
	}
	executed, failed := counter.Counts(exit.Code)
	record := &run.Record{
		Filter:      filter,
		Paths:       paths,
		ExitCode:    exit.Code,
		TimedOut:    exit.TimedOut,
		Unstarted:   unstarted,
		DurationMS:  took.Milliseconds(),
		TestsRun:    executed,
		TestsFailed: failed,
		OutputTail:  tail.String(),
		Sandbox:     s.generation,
	}
	if unstarted {
		// A runner that never started returned no status, and the zero
		// RunExit hands back would store one that reads as success.
		record.ExitCode = -1
	}
	return &measuredRun{record: record, unstarted: unstarted, detail: detail}, nil
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

// newProbeRunCmd performs one probe of §5.3 or §5.4 and records the result
// (§11).
//
// Each kind's cycle is §5.3.2's or §5.4.2's own sentence, and in both the undo
// is not the last step of it but the frame around the middle two:
// state.UnderSandboxMutation and state.UnderSandboxTestFile write, call the
// run, and put the sandbox back whether the run returned, failed, timed out, or
// panicked. §5.3.3 and §5.4.2 admit no path where the artefact stays on disk
// that cr is still alive to prevent, and an undo written after the run would be
// one `return err` away from being skipped.
func newProbeRunCmd(out *writer) *cobra.Command {
	var kind, patchFile, testFile, filter, target, fromProposal, rerun string
	var paths []string
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
			// §5.2.1's three conditions on a `--path`, which §5.3.2
			// and §5.4.2 pass through this command, asked before any
			// state is read: §11.2 codes a malformed invocation 2,
			// and such a run must have executed nothing.
			if err := run.CheckPaths(paths); err != nil {
				return err
			}
			request := &probeRequest{
				owner: owner, repo: repo, pr: pr,
				filter: filter, paths: paths, target: target,
			}
			ran, err := performProbe(cmd, out, request, &probeFlags{
				kind: kind, patchFile: patchFile, testFile: testFile,
				target: target, filter: filter, paths: paths,
				fromProposal: fromProposal, rerun: rerun,
			})
			if err != nil {
				return err
			}
			// Both kinds read the round's head from the clone, to build the
			// sandbox and to place the probe, so a head the clone lacks is
			// the fetch it is.
			return headNotFetched(cmd, owner, repo, pr, ran)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "",
		"the probe to run: mutation (§5.3) or gap (§5.4)")
	cmd.Flags().StringVar(&patchFile, "patch", "",
		"the unified diff to apply, per §5.3.1; a mutation probe only")
	cmd.Flags().StringVar(&testFile, "test", "",
		"the new test file to place and run, per §5.4.1; a gap probe only")
	cmd.Flags().StringVar(&filter, "filter", "",
		"narrow the run to a subset, passed as the profile's tests.filter_flag")
	cmd.Flags().StringArrayVar(&paths, "path", nil,
		"narrow the run to a path inside the sandbox, passed through the profile's "+
			"tests.paths_arg; repeatable, and the population the baseline measures")
	cmd.Flags().StringVar(&target, "target", "",
		"the path:line the probe addresses; required for a gap probe, "+
			"rejected for a mutation probe, which derives it (§5.3.2)")
	cmd.Flags().StringVar(&fromProposal, "proposal", "",
		"run the experiment a role proposed, by its x<n> id (§5.7.3); "+
			"every other input flag is rejected beside it")
	cmd.Flags().StringVar(&rerun, "rerun", "",
		"re-run a stored probe at the current head, by its p<n> id (§5.5.4); "+
			"every other input flag is rejected beside it")
	return cmd
}

// onlyTheProposal is §5.7.3's refusal of an input flag beside `--proposal`,
// with exit code 2: the proposal carries every input, and a flag beside it
// would run something other than what the role proposed.
func onlyTheProposal(kind, patchFile, testFile, target, filter string, paths []string) error {
	for _, given := range []struct{ flag, value string }{
		{"--kind", kind}, {"--patch", patchFile}, {"--test", testFile},
		{"--target", target}, {"--filter", filter},
	} {
		if given.value != "" {
			return fmt.Errorf("%s is rejected beside --proposal: §5.7.3 runs the proposal's own "+
				"kind, input, target, filter and paths, so the experiment cr executes is the one "+
				"the role proposed", given.flag)
		}
	}
	if len(paths) > 0 {
		return errors.New("--path is rejected beside --proposal: §5.7.3 runs the proposal's own " +
			"kind, input, target, filter and paths, so the experiment cr executes is the one " +
			"the role proposed")
	}
	return nil
}

// probeFlags is `cr probe run`'s command line, as the flags gave it.
//
// It is a struct rather than eight parameters because every one of them is read
// by one decision — which kind runs, and out of what — and a positional list
// that long is where a caller swaps `target` for `filter` and nothing says so.
type probeFlags struct {
	kind, patchFile, testFile string
	target, filter            string
	paths                     []string
	// fromProposal is §5.7.3's `--proposal`, empty for a run the command
	// line supplied whole.
	fromProposal string
	// rerun is §5.5.4's `--rerun`, empty for a run that re-runs nothing.
	rerun string
}

// performProbe resolves the run's inputs and performs it, returning what the
// probe run reported so the caller can route a git failure through
// headNotFetched.
//
// §5.7.3's proposal and §5.5.4's re-run supply every input, so the two input
// readers are skipped for them: they hold a command line to the flags its kind
// takes, and §5.7.3 and §5.5.4 admit none.
func performProbe(
	cmd *cobra.Command, out *writer, request *probeRequest, flags *probeFlags,
) (ran, refused error) {
	kind := flags.kind
	switch {
	case flags.rerun != "":
		// Asked first, so `--proposal` beside `--rerun` is refused as
		// one of the flags §5.5.4 rejects rather than run as a proposal.
		rerunKind, err := rerunRun(request, flags)
		if err != nil {
			return nil, err
		}
		kind = rerunKind
	case flags.fromProposal != "":
		proposed, err := proposedRun(
			request.owner, request.repo, request.pr, flags.fromProposal, request,
			flags.kind, flags.patchFile, flags.testFile, flags.target, flags.filter, flags.paths)
		if err != nil {
			return nil, err
		}
		kind = proposed
	}
	fromFlags := request.proposal == "" && request.rerunOf == ""
	switch probe.Kind(kind) {
	case probe.Mutation:
		if fromFlags {
			if err := mutationInput(request, flags.patchFile, flags.testFile, flags.target); err != nil {
				return nil, err
			}
		}
		return runMutationProbe(cmd, out, request), nil
	case probe.Gap:
		if fromFlags {
			if err := gapInput(request, flags.patchFile, flags.testFile, flags.target); err != nil {
				return nil, err
			}
		}
		return runGapProbe(cmd, out, request), nil
	}
	return nil, fmt.Errorf(
		"--kind %q: §5.5 names two kinds of probe, mutation (§5.3) and gap (§5.4)", kind)
}

// mutationInput holds §5.3's invocation to the flags that kind takes, and reads
// the patch.
//
// §5.3.2 rejects `--target`, and the refusal is the point rather than a
// tidiness: the target is what §6.2.2 has a `probed` record's evidence point
// at, so a supplied one would let the agent aim the assertion at a line the
// experiment never touched. Accepting and ignoring it would be worse than
// either — the agent would have named a line and been told nothing. `--test` is
// refused for the same reason in the other direction: a mutation probe places
// no file, so a supplied one would never be run.
func mutationInput(request *probeRequest, patchFile, testFile, target string) error {
	switch {
	case target != "":
		return errors.New(
			"--target is rejected for a mutation probe: §5.3.2 derives it from the patch, " +
				"so the evidence chain runs on what cr executed rather than on a flag")
	case testFile != "":
		return errors.New(
			"--test is rejected for a mutation probe: §5.3.1 supplies the experiment as a " +
				"unified diff, and §5.4's gap probe is the kind that places a test file")
	case patchFile == "":
		return errors.New(
			"--patch is required: §5.3.1 has the agent supply the mutation as a unified diff")
	}
	body, err := readInput(patchFile,
		"`--patch` names the file holding §5.3.1's unified diff; check the path")
	if err != nil {
		return err
	}
	files, err := git.ParsePatch(string(body))
	if malformed := (*git.MalformedPatchError)(nil); errors.As(err, &malformed) {
		// ParsePatch never learns the file; §11.2's refusal names it.
		malformed.File = patchFile
	}
	if err != nil {
		return err
	}
	if len(files) == 0 {
		// The hint names the flag rather than the concept, because
		// the way this is reached is a `git diff` on a machine whose
		// owner has set `diff.external`: git then writes that tool's
		// output, which is not a diff at all. cr's own reads pin
		// `--no-ext-diff` for the same reason, and the agent writing
		// the patch is outside that fence.
		return &git.MalformedPatchError{File: patchFile, Problem: "holds no hunk: " +
			"§5.3.1's mutation is a unified diff against a sandbox file; " +
			"if it came from `git diff`, re-run it with --no-ext-diff, " +
			"which is what a configured diff.external replaces"}
	}
	request.kind, request.patch, request.files = probe.Mutation, string(body), files
	request.patchFile = patchFile
	return nil
}

// absentFromPatchStep is §12.4's next step for a patch naming a file the
// sandbox does not hold.
const absentFromPatchStep = "correct the file header on the line the message names so it names a file the " +
	"pull request's head holds, as a path from the repository root"

// absentFromPatch words a path the sandbox holds no file at as a refusal of the
// patch line that named it, and passes every other refusal through.
//
// The sandbox check reports the path; the reader has the patch file open, so
// the refusal names that file and the +++ header's line, as every other
// refusal of the patch's data does, and takes §11.2's 1 with it.
func absentFromPatch(patchFile string, file *git.PatchedFile, err error) error {
	if absent := (*state.AbsentFromSandboxError)(nil); !errors.As(err, &absent) {
		return err
	}
	return &git.MalformedPatchError{
		File: patchFile, Line: file.HeaderLine,
		Problem: fmt.Sprintf("the file header names %q, which is not a file the sandbox holds: "+
			"a probe applies hunks only to files already there, and a file it created would have "+
			"no original to put back", file.Path),
		Step: absentFromPatchStep,
	}
}

// gapInput holds §5.4's invocation to the flags that kind takes, and reads the
// test file.
//
// `--target` is required rather than derived, and that is §5.5's own asymmetry:
// a mutation probe's target follows from the patch, while a gap probe's test
// file names no line of production code at all, so the line the finding is
// anchored at is the agent's to supply. `--patch` is refused because §5.4.1
// supplies the experiment as a test file, and a patch cr accepted and never
// applied would be an experiment the agent believed it had asked for.
func gapInput(request *probeRequest, patchFile, testFile, target string) error {
	switch {
	case patchFile != "":
		return errors.New(
			"--patch is rejected for a gap probe: §5.4.1 has the agent supply a new test file, " +
				"and §5.3's mutation probe is the kind that applies a diff")
	case testFile == "":
		return errors.New(
			"--test is required: §5.4.1 has the agent supply a new test file targeting the " +
				"suspected edge case")
	case target == "":
		return errors.New(
			"--target is required for a gap probe: §5.5 makes the probe record's target a " +
				"required row and takes it from this flag, because a supplied test file " +
				"names no line of the code under review")
	}
	// §5.5's shape, asked before any file is opened and any state is
	// touched: a `--target` that is not a `path:line` names nowhere at all,
	// and the record's row is required whatever the head turns out to hold.
	// Whether the head holds the location is asked later, where the round's
	// head is resolved.
	if _, _, err := probe.ParseTarget(target); err != nil {
		return err
	}
	body, err := readInput(testFile,
		"`--test` names the file holding §5.4.1's new test; check the path")
	if err != nil {
		return err
	}
	request.kind, request.test = probe.Gap, string(body)
	return nil
}

// probeRequest is one `cr probe run` invocation, as the command line gave it.
type probeRequest struct {
	owner string
	repo  string
	pr    int
	// kind is §5.5's `kind`: which of §5.3 and §5.4 is being performed.
	kind probe.Kind
	// patch is the mutation diff as it was handed over, which §5.5's
	// `input` row stores whole: the record has to say which experiment
	// was performed.
	patch string
	// patchFile is the `--patch` path the diff was read from, which a
	// refusal of the patch's data names.
	patchFile string
	// files is the same diff, parsed for application.
	files []git.PatchedFile
	// test is the gap probe's test file content, which §5.5's `input` row
	// stores whole for the same reason.
	test string
	// target is §5.5's `target` for a gap probe, supplied by the agent.
	target string
	// filter is what the run is narrowed to, empty for the whole suite.
	filter string
	// paths are the `--path` values, empty for a run of every test the
	// filter selects. §5.2.2 makes them the population the probe's
	// baseline measures, and §5.5 stores them on the record.
	paths []string
	// proposal is the §5.7 proposal this run executes, empty for a run the
	// command line supplied. §5.7.4 settles it once the probe is written.
	proposal string
	// rerunOf is the probe §5.5.4's `--rerun` re-runs, empty otherwise,
	// and the record's `rerun_of`.
	rerunOf string
}

// probeSetup is the round, the profile, and the sandbox one probe run works
// against, resolved once and the same way for both kinds.
//
// It is shared rather than duplicated because every field in it is something
// the two kinds must agree about: the round's head, the profile `cr brief`
// resolved, the sandbox §5.1.6 admitted, and the leftover glob that check reads.
// Two copies of this resolution would be two answers that could drift.
type probeSetup struct {
	layout state.Layout
	round  state.Meta
	// dir is the repository under review, which §5.6.1 names the lock
	// after.
	dir string
	// src and glob are §5.1.6's inputs, kept for the post-run check.
	src  *sandbox.Sources
	glob string
	// ready is the sandbox the check admitted, with its recreation notice.
	ready *sandbox.Ready
	// tests is the runner, resolved once for every run the probe performs.
	tests *suite
	// stamp is §2.3.3's head and round, written onto every record.
	stamp state.Stamp
	// announce prints the experiment header for the probe's own argv, and
	// for §5.2.2's unfiltered baseline argv when that runs first, and
	// returns its env-file sentences.
	announce func(command, baseline []string) ([]string, error)
	// uncopied are those sentences, kept for the document's honesty.
	uncopied []string
	// capped is §5.6.4's budget as it stood before this run, kept so the
	// run that fills it can say so. It is measured in prepareProbe, because
	// the refusal has to land before anything is done in the sandbox, and
	// measured again by underProbeLock once §5.6.1's lock is held, which is
	// the count this field carries from then on.
	capped probe.RoundCap
}

// prepareProbe resolves the round, the profile, and the sandbox a probe runs
// in.
//
// The profile is the round's resolved one, for the reason `cr test` reads the
// same field: §3.7 makes `cr brief` its one writer, and a probe that
// re-selected one could measure a suite this round was never briefed against.
//
// §5.6.4's cap is asked here, and before the sandbox is ensured rather than
// after. A refused run must execute nothing in the sandbox, and `sandbox.Ensure`
// is already execution: §5.1.3 runs the profile's setup commands, and a
// recreation would run them for a probe cr was never going to perform.
func prepareProbe(cmd *cobra.Command, out *writer, request *probeRequest) (*probeSetup, error) {
	layout, err := state.Default()
	if err != nil {
		return nil, err
	}
	round, err := briefedRound(layout, request.owner, request.repo, request.pr)
	if err != nil {
		return nil, err
	}
	// §9.3.2: a probe writes its record against the round and mutates the
	// sandbox checked out at the round's head, so a head that moved under
	// the round refuses here — before §5.6.4's cap is spent and before
	// §5.1.3's setup commands can run.
	if err := round.RefuseStale(); err != nil {
		return nil, err
	}
	dir, err := repoDir()
	if err != nil {
		return nil, err
	}
	capped, err := probeBudget(layout, request.owner, request.repo, request.pr, round.Round)
	if err != nil {
		return nil, err
	}
	resolved, file, err := roundProfile(layout, round.ProfileID)
	if err != nil {
		return nil, err
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
		Require:     resolved.Sandbox.Require,
		ProfileFile: file,
	}
	glob := resolved.LeftoverGlob()
	ready, err := sandbox.Ensure(src, glob)
	if err != nil {
		return nil, err
	}
	// §5.1.8, in the order the section gives it: after §5.1.6's check, and
	// before any run — which for a probe means before §5.2.6's baseline as
	// well as before the probe's own run, since both measure the sandbox
	// this refuses to run a suite in.
	if err := sandbox.CheckRequired(src, ready.Path); err != nil {
		return nil, err
	}
	return &probeSetup{
		layout: layout,
		round:  round.Meta,
		dir:    dir,
		src:    src,
		glob:   glob,
		ready:  ready,
		tests: &suite{
			// §11.1: the live echo is informational and `--quiet`
			// takes it; the tail and the counter in run() see the
			// stream either way.
			profile: resolved, file: file, path: ready.Path, src: src, generation: ready.Generation,
			log: out.informational(cmd.ErrOrStderr()),
		},
		announce: func(command, baseline []string) ([]string, error) {
			return announceExperiment(cmd, out, src, ready, command, baseline)
		},
		stamp:  state.Stamp{Head: round.Head, Round: round.Round},
		capped: capped,
	}, nil
}

// probeBudget measures §5.6.4's cap over the round and refuses a run that would
// exceed it.
//
// The count comes from probes.ndjson itself rather than from a counter kept
// beside it: §2.3.3 stamps `round` on every record, so the file answers the
// question, and a separate tally would be a second answer that could drift from
// the records it claims to summarise. The read is lock-free per §2.3.2.
func probeBudget(
	l state.Layout, owner, repo string, pr, round int,
) (probe.RoundCap, error) {
	stored, err := state.ReadStamped[probe.Record](l, owner, repo, pr, state.FileProbes, round)
	if err != nil {
		return probe.RoundCap{}, err
	}
	capped, err := roundCap(l, owner, repo, stored, round)
	if err != nil {
		return probe.RoundCap{}, err
	}
	return capped, capped.Err()
}

// roundCap measures stored against the resolved `probe.max_per_round`, so the
// count before a run, the count under §5.6.1's lock and the count at the
// record's write are one measurement taken at three moments.
func roundCap(
	l state.Layout, owner, repo string, stored []probe.Record, round int,
) (probe.RoundCap, error) {
	resolved, err := config.Resolve(config.Sources{
		Environ:      os.Environ(),
		GlobalConfig: l.Config(),
		RepoConfig:   l.RepoConfig(owner, repo),
	})
	if err != nil {
		return probe.RoundCap{}, err
	}
	return probe.RoundCapFor(stored, round, resolved.Int("probe.max_per_round")), nil
}

// underProbeLock performs a probe's locked half inside §5.6.1's lock, and
// returns what it stored with §5.6.3's warning.
//
// §5.6.4's cap is counted again once the lock is held, and the lock is held
// until locked has written the record. prepareProbe's count is taken before the
// lock, so every run started while another was waiting or running passes it;
// only a count taken under the lock, with the previous holder's record already
// on disk, sees what the runs before it spent. A run refused here has performed
// no suite.
//
// The lock is taken once around every run this command performs. The baselines
// of §5.2.6 run the same suite against the same test database as the probe
// does, so a lock taken per run would either leave the gaps between them open
// or, taken twice over, wait on itself for the whole of
// `probe.lock_timeout_seconds`.
func underProbeLock(
	setup *probeSetup, request *probeRequest, locked func() (*finishedProbe, error),
) (*finishedProbe, string, error) {
	held, err := lockProbe(
		setup.layout, request.owner, request.repo, setup.dir, setup.round.ProfileID)
	if err != nil {
		return nil, "", err
	}
	var finished *finishedProbe
	capped, err := probeBudget(setup.layout, request.owner, request.repo, request.pr, setup.stamp.Round)
	if err == nil {
		setup.capped = capped
		finished, err = locked()
	}
	if err := errors.Join(err, held.Unlock()); err != nil {
		return nil, "", err
	}
	return finished, held.CollisionWarning(), nil
}

// runMutationProbe performs §5.3.2's cycle and records what it produced.
func runMutationProbe(cmd *cobra.Command, out *writer, request *probeRequest) error {
	// §5.3.2's target, derived from the patch before anything is run. A
	// patch cr cannot place a target in is refused here rather than after
	// §5.2.6 has performed a baseline suite for it, and §5.5 makes the
	// target a required row of the record either way.
	aimed, err := probe.Target(request.files)
	if err != nil {
		return err
	}
	setup, err := prepareProbe(cmd, out, request)
	if err != nil {
		return err
	}
	// Round 12's unbounded-patch-target: every path the patch addresses
	// has to land inside the sandbox. It is asked here rather than left to
	// the writer, so a diff aimed out of the tree is refused as the input
	// it is instead of being met after a baseline suite has already run —
	// and it is the writer's own function, so the two cannot disagree.
	for i := range request.files {
		if _, err := setup.layout.InSandbox(
			request.owner, request.repo, request.pr, request.files[i].Path); err != nil {
			return absentFromPatch(request.patchFile, &request.files[i], err)
		}
	}

	finished, warning, err := underProbeLock(setup, request, func() (*finishedProbe, error) {
		performed, measured, err := mutationRuns(setup, request)
		if err != nil {
			return nil, err
		}
		// §5.1.6's check, now that the suite has finished and the
		// mutation is off disk again. A check run before the revert
		// would find every probe's own mutation and void every probe.
		unclean, err := sandbox.Unclean(setup.src, setup.glob)
		if err != nil {
			return nil, err
		}
		outcome := probe.Decide(probe.Ladder(measured), unclean)
		return storeProbe(setup, request, &finishedProbe{
			performed: performed, outcome: outcome, unclean: unclean,
			establishes: establishedBy(outcome, performed.baseline),
			record: &probe.Record{
				Kind:        probe.Mutation,
				Input:       request.patch,
				Filter:      request.filter,
				Paths:       request.paths,
				Result:      outcome.Result(),
				Reason:      outcome.Reason(probe.Reason(measured)),
				TestsRun:    measured.TestsRun,
				TestsFailed: measured.TestsFailed,
				Target:      aimed,
				Baseline:    performed.baseline.ID(),
				DurationMS:  performed.durationMS,
				OutputTail:  performed.outputTail,
				RerunOf:     request.rerunOf,
			},
		})
	})
	if err != nil {
		return err
	}
	return reportProbe(out, setup, request, finished, warning)
}

// runGapProbe performs §5.4.2's cycle and records what it produced.
//
// The probe's id is allocated before the placement and not at the write, and
// that is §5.4.2's template rather than a convenience: §2.4 puts `<probe-id>`
// in the path, and §5.1.6 recognises a leftover artefact by replacing exactly
// that position. A file named for an id no record carries would be an artefact
// nothing accounts for.
func runGapProbe(cmd *cobra.Command, out *writer, request *probeRequest) error {
	setup, err := prepareProbe(cmd, out, request)
	if err != nil {
		return err
	}
	// §5.5: a gap probe's target is supplied and validated as §6.2.3
	// validates a citation, against the head this round was briefed at.
	// It is asked here, before §5.2.6 performs a baseline suite and before
	// the lock is taken, because a record whose `target` names a location
	// nobody can open is one §6.2.2 could never grade a finding from — and
	// the experiment would have been performed for nothing.
	if err := probe.CheckTarget(func(path string) ([]string, bool, error) {
		return git.FileAtRevision(setup.dir, setup.round.Head, path)
	}, request.target); err != nil {
		return err
	}
	stored, err := state.ReadRecords[probe.Record](
		setup.layout, request.owner, request.repo, request.pr, state.FileProbes)
	if err != nil {
		return err
	}
	reserved := probe.NextID(stored)
	placement, err := gapPlacement(setup, request, reserved)
	if err != nil {
		return err
	}
	finished, warning, err := underProbeLock(setup, request, func() (*finishedProbe, error) {
		performed, measured, err := gapRuns(setup, request, placement)
		if err != nil {
			return nil, err
		}
		// §5.1.6's check, now that the suite has finished and the probe
		// file is off disk again. A check run before the removal would
		// find every gap probe's own test file and void every gap probe.
		unclean, err := sandbox.Unclean(setup.src, setup.glob)
		if err != nil {
			return nil, err
		}
		outcome := probe.Decide(probe.GapLadder(measured), unclean)
		record := &probe.Record{
			ID:          reserved,
			Kind:        probe.Gap,
			Input:       request.test,
			Filter:      request.filter,
			Paths:       request.paths,
			Result:      outcome.Result(),
			Reason:      outcome.Reason(probe.GapReason(measured)),
			TestsRun:    measured.TestsRun,
			TestsFailed: measured.TestsFailed,
			Target:      request.target,
			Baseline:    performed.baseline.ID(),
			DurationMS:  performed.durationMS,
			OutputTail:  performed.outputTail,
			RerunOf:     request.rerunOf,
		}
		return storeProbe(setup, request, &finishedProbe{
			performed: performed, record: record, outcome: outcome, unclean: unclean,
			establishes: gapEstablishedBy(record),
		})
	})
	if err != nil {
		return err
	}
	return reportProbe(out, setup, request, finished, warning)
}

// gapPlacement resolves the sandbox path a gap probe reserved as id places its
// test file at, and refuses one it cannot place.
func gapPlacement(setup *probeSetup, request *probeRequest, reserved string) (string, error) {
	// §5.4.2's `<target-dir>`: the directory of the path `--target` names.
	// probe.ParseTarget has already held the target to §5.5's shape, so the
	// path here is one cr resolved against the head.
	targetPath, _, err := probe.ParseTarget(request.target)
	if err != nil {
		return "", err
	}
	placement := setup.tests.profile.ProbePath(reserved, path.Dir(targetPath))
	if placement == "" {
		return "", &profile.MalformedError{
			File:  setup.tests.file,
			Field: "tests.probe_path_template",
			Problem: "resolves to no path, so §5.4.2 has nowhere to place a gap probe's test; " +
				"set it, or set tests.globs for it to be defaulted from",
		}
	}
	// §5.4.2's abort, asked before §5.2.6 performs a baseline suite for a
	// probe that cannot be placed. It is the writer's own function, so the
	// answer here and the answer the placement is held to cannot disagree.
	if _, err := setup.layout.FreeInSandbox(
		request.owner, request.repo, request.pr, placement); err != nil {
		return "", err
	}
	if err := underProbeDirectory(request.paths, placement); err != nil {
		return "", err
	}
	return placement, nil
}

// underProbeDirectory is §5.4.2's other abort: when `--path` is given, at least
// one of them must lie under the directory the placement sits in.
//
// It is the condition that keeps the two halves of a gap probe comparable.
// §5.4.2 narrows the probe's own run to the file cr placed, and §5.2.2 has its
// baseline measure the `--path` values instead — so paths that reach none of
// the directory the probe file goes in produce a baseline of one population and
// a probe run of another, and the comparison §5.4.3's ladder makes between them
// would be a comparison of two different suites.
//
// "Under" includes the directory itself, which is the ordinary invocation: a
// reviewer narrowing a Laravel gap probe writes `--path tests/Feature`, the
// directory `tests/Feature/cr_probe_<probe-id>Test.php` sits in. A template
// whose path has no directory of its own puts every path under it, since
// §5.2.1 already holds each one inside the sandbox.
//
// The refusal is a malformed invocation and takes §11.2's 2: the command line
// named a set of paths that cannot answer the question it also asked.
func underProbeDirectory(paths []string, placement string) error {
	if len(paths) == 0 {
		return nil
	}
	dir := filepath.Dir(filepath.FromSlash(placement))
	if dir == "." {
		return nil
	}
	for _, path := range paths {
		cleaned := filepath.Clean(path)
		if cleaned == dir || strings.HasPrefix(cleaned, dir+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf(
		"no --path lies under %s, which is where tests.probe_path_template puts this probe's "+
			"test file (%s): §5.4.2 runs the probe over the file it placed and §5.2.2 measures its "+
			"baseline over the --path values, so paths that reach none of that directory compare "+
			"two different populations", dir, placement)
}

// probeRunPath is the path §5.4.2 gives the probe's own run: the directory the
// placed file sits in, `.` included.
//
// The repository root is a directory like any other, and §5.4.2 makes no
// exception for it. Measured 2026-09-22 on a Go module whose package is at the
// root: v0.6.0 handed the root-placed file itself to `go test`, which compiled
// it alone as `command-line-arguments`, failed on the undefined function it
// tested, and left every such gap probe `inconclusive` — measurement 4's
// symptom again, one directory up. `go test ./.` runs the root package.
func probeRunPath(placement string) string {
	return path.Dir(placement)
}

// finishedProbe is what a probe's locked half hands to its report: the stored
// record, what it was decided from, and the ids its write allocated.
type finishedProbe struct {
	performed *performedProbe
	record    *probe.Record
	outcome   probe.Outcome
	// unclean is §5.1.6's post-run answer, and establishes what the result
	// may be read for.
	unclean, establishes string
	// runID and probeID are what recordProbe allocated.
	runID, probeID string
}

// storeProbe writes §5.5's record and applies §5.1.7's third consequence.
//
// It is shared by both kinds because none of it differs between them: the
// contamination flag and the ordering of the record against the recreation are
// the same acts on the same values. What differs — which ladder read the run,
// and what the result may be read for — is settled by the caller before it gets
// here. It runs inside §5.6.1's lock, so the record §5.6.4 counts has landed
// before the next run holding the lock counts it.
func storeProbe(
	setup *probeSetup, request *probeRequest, finished *finishedProbe,
) (*finishedProbe, error) {
	if finished.performed.underProbe != nil {
		finished.performed.underProbe.Contaminated = finished.unclean != ""
	}
	runID, probeID, err := recordProbe(
		setup.layout, request.owner, request.repo, request.pr,
		setup.stamp, finished.performed.underProbe, finished.record)
	if err != nil {
		return nil, err
	}
	finished.runID, finished.probeID = runID, probeID
	// §5.1.7: a voided probe forces recreation before the next run. It
	// happens after the record lands, so the evidence the check found is
	// on disk before the sandbox it describes is rebuilt.
	if finished.outcome.Voided() {
		if err := sandbox.ForceRecreation(setup.src, fmt.Sprintf(
			"probe %s was voided after its run: %s", probeID, finished.unclean)); err != nil {
			return nil, err
		}
	}
	return finished, nil
}

// reportProbe emits what a stored probe run produced, once §5.6.1's lock has
// been released.
func reportProbe(
	out *writer, setup *probeSetup, request *probeRequest, finished *finishedProbe, warning string,
) error {
	// §5.7.4, before the report: the proposal that asked for this
	// experiment becomes `run` and the record it named is re-graded, so what
	// is emitted below is the state the round is actually in.
	var regraded *Regraded
	if request.proposal != "" {
		settled, err := settleProposal(
			setup.layout, request.owner, request.repo, request.pr,
			&setup.round, request.proposal, finished.probeID)
		if err != nil {
			return err
		}
		regraded = settled
	}
	return out.emit(&probeRunResult{
		Proposal:    request.proposal,
		Regraded:    regraded,
		RerunOf:     request.rerunOf,
		Probe:       finished.probeID,
		Kind:        string(finished.record.Kind),
		Sandbox:     setup.ready.Path,
		Command:     finished.performed.command,
		Filter:      request.filter,
		Paths:       request.paths,
		Result:      string(finished.outcome.Result()),
		Reason:      finished.record.Reason,
		Establishes: finished.establishes,
		Target:      finished.record.Target,
		Baseline:    finished.performed.baseline.ID(),
		Run:         finished.runID,
		Voided:      finished.unclean,
		Warnings:    []string{warning},
		Honesty: append(append(probeDisclosures(setup, finished), setup.uncopied...),
			setup.tests.profile.StaleDisclosures()...),
	})
}

// probeDisclosures is what §11.1 exempts from `--quiet` on a probe run: §5.1.6's
// recreation notice, and §5.6.4's cap when this run is the one that reached it.
//
// The cap is disclosed at the moment it is reached and not on every run, and
// §5.6.4 is why in both directions. "The cap being hit MUST be reported" is the
// obligation, so the run that hits it says so; "never silently applied" is what
// that report is for, and a reader who is told after the tenth probe that the
// round has no budget left learns it before the eleventh refuses rather than
// from the refusal. A line on every run would say nothing new nine times over
// and would push the one that matters into the noise.
//
// The count disclosed is the round's after this run, because the number the
// reader needs is how many probes the round has now spent.
//
// A probe §5.1.7 voided says so too, naming what the post-run check found. Its
// `error` is not the ladder's, and a reader told only `error` cannot tell a
// suite that writes into tracked files from a runner that broke.
//
// So does a probe whose result would have counted but for its baseline (field
// feedback 1.7): on a suite that already fails, `establishes` reads `nothing`
// beside a `no-test-failed`, and nothing else in the document says why.
func probeDisclosures(setup *probeSetup, finished *finishedProbe) []string {
	disclosed := recreationNotice(setup.ready)
	if finished.outcome.Voided() {
		disclosed = append(disclosed, fmt.Sprintf(
			"probe %s voided, per §5.1.7: %s; its result is error, it grades no finding, "+
				"and the sandbox is recreated before the next run", finished.probeID, finished.unclean))
	}
	if unfounded := unpassedBaseline(finished); unfounded != "" {
		disclosed = append(disclosed, unfounded)
	}
	if spent := setup.capped.Ran(); spent.Reached() {
		disclosed = append(disclosed, spent.Disclosure())
	}
	if measured := finished.performed.underProbe; measured != nil {
		disclosed = append(disclosed, measured.Uncounted()...)
	}
	return append(disclosed, survivorNotice(setup.tests.lingering)...)
}

// unpassedBaseline is the disclosure of a probe whose result is the one its
// kind's section reads — §5.3.5's `no-test-failed`, §5.4.4's `failed` — over a
// baseline that did not pass per §5.2.5, naming that baseline and its failed
// count, and empty for every other probe. Every other result establishes the
// same whatever the baseline, so naming the baseline there would blame it for
// nothing.
func unpassedBaseline(finished *finishedProbe) string {
	baseline := finished.performed.baseline
	if baseline.Passed() {
		return ""
	}
	failed := "no tests_failed was derivable"
	if count, counted := baseline.Failed(); counted {
		failed = fmt.Sprintf("tests_failed %d", count)
	}
	switch {
	case probe.Proven(finished.record):
		return fmt.Sprintf("probe %s establishes no gap: its baseline run %s did not pass per §5.2.5 (%s), "+
			"and §5.3.5 lets no-test-failed prove a gap only over a baseline that passed",
			finished.probeID, baseline.ID(), failed)
	case probe.Reproduces(finished.record):
		return fmt.Sprintf("probe %s supports no finding: its baseline run %s did not pass per §5.2.5 (%s), "+
			"and §5.4.4 lets a failed gap probe support a finding only over a baseline that passed",
			finished.probeID, baseline.ID(), failed)
	}
	return ""
}

// performedProbe is everything the locked half of a probe produced.
type performedProbe struct {
	// baseline is the run §5.2.6 resolved, performing it first when the
	// head had none.
	baseline probe.Baseline
	// underProbe is §5.2.4's record for the run on mutated or
	// probe-injected code, and nil when no such run happened — a mutation
	// patch that did not apply runs no tests.
	underProbe *run.Record
	// command is the argv the probe's own run was to be started with,
	// which is reported whether or not it ran.
	command []string
	// durationMS and outputTail are §5.5's rows for the probe's own run.
	durationMS int64
	outputTail string
}

// probeBaselines performs and records every baseline §5.2.2 requires that the
// head and sandbox generation have no run for yet (§5.2.6), before the probe
// itself runs.
//
// The baselines come first and never after, which is what keeps them un-probed:
// they run against the sandbox as §5.1 prepared it, so the record §5.2.6 stores
// is admissible by construction rather than by inspection.
//
// ownPaths are the paths the probe's own run is narrowed to, which are not
// always the paths its baseline measures: §5.4.2 narrows a gap probe's run to
// the one file cr placed, and §5.2.2 has its baseline measure the `--path`
// values instead, because the placed file does not exist in the baseline.
func probeBaselines(
	setup *probeSetup, request *probeRequest, ownPaths []string,
) (*performedProbe, error) {
	command, err := setup.tests.profile.TestArgv(setup.tests.file, request.filter, ownPaths)
	if err != nil {
		return nil, err
	}
	performed := &performedProbe{command: command}

	stored, err := state.ReadRecords[run.Record](
		setup.layout, request.owner, request.repo, request.pr, state.FileRuns)
	if err != nil {
		return nil, err
	}
	// Only the runs of the sandbox this probe runs in can be its baselines.
	stored = probe.OfSandbox(stored, setup.ready.Generation)
	// The header goes out before the first run, and says what §5.2.2's
	// baseline is when that baseline is the run about to happen: an
	// operator who asked for one narrowed experiment may be about to wait
	// for the entire suite, and that is worth saying before the wait
	// rather than after it.
	referenced := probe.Referenced(request.kind, request.filter, request.paths)
	var baseline []string
	if len(probe.Missing(stored, setup.round.Head,
		probe.Required(request.kind, request.filter, request.paths))) > 0 {
		if baseline, err = setup.tests.profile.TestArgv(
			setup.tests.file, referenced.Filter, referenced.Paths); err != nil {
			return nil, err
		}
	}
	if setup.uncopied, err = setup.announce(command, baseline); err != nil {
		return nil, err
	}
	// unclean is what §5.1.6's check found after a baseline this run
	// performed, kept for the refusal when that baseline does not stand.
	var unclean string
	performed.baseline, err = probe.Ensure(
		stored, setup.round.Head, request.kind, request.filter, request.paths,
		func(spec probe.Spec) (run.Record, error) {
			ran, found, err := baselineRun(setup, request, spec)
			if found != "" {
				unclean = found
			}
			return ran, err
		})
	if unusable := (*probe.NoBaselineError)(nil); errors.As(err, &unusable) {
		return nil, unusableBaseline(setup, request, unclean, err)
	}
	if err != nil {
		return nil, err
	}
	return performed, nil
}

// unusableBaseline is the refusal of a probe whose freshly performed baseline
// does not stand: §5.1.6's check failed after it, so it measured a sandbox
// that had drifted from the head and is nobody's baseline.
//
// It is a failure of the sandbox the command used, which §11.2 codes 3, and the
// step names that sandbox and the command that rebuilds it. Nothing about the
// command line was wrong, so it is not the malformed invocation an unmapped
// error would read as.
func unusableBaseline(setup *probeSetup, request *probeRequest, unclean string, err error) error {
	if unclean != "" {
		err = fmt.Errorf("%w; §5.1.6's check after that run found that %s", err, unclean)
	}
	return state.FileFailure("use", setup.ready.Path, fmt.Sprintf(
		"the baseline run left the sandbox %s unclean, so it measured nothing; run "+
			"`cr sandbox destroy %d --repo %s/%s`, keep the suite from leaving that behind, "+
			"and run the probe again",
		setup.ready.Path, request.pr, request.owner, request.repo), err)
}

// mutationRuns performs §5.2.6's baselines and then the mutated run, all inside
// §5.6.1's lock.
func mutationRuns(
	setup *probeSetup, request *probeRequest,
) (*performedProbe, probe.Measured, error) {
	performed, err := probeBaselines(setup, request, request.paths)
	if err != nil {
		return nil, probe.Measured{}, err
	}

	mutations := make([]state.SandboxMutation, 0, len(request.files))
	for i := range request.files {
		file := &request.files[i]
		mutations = append(mutations, state.SandboxMutation{Path: file.Path, Apply: file.Apply})
	}
	var mutated *measuredRun
	err = setup.layout.UnderSandboxMutation(
		request.owner, request.repo, request.pr, mutations, func() error {
			ran, err := setup.tests.perform(request.filter, request.paths)
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
		// say that the mutation was never made. The zero Measured is
		// that run: Applied is false, which is the rung itself, and
		// the refusal is what the record's reason names.
		return performed, probe.Measured{Detail: refused.Error()}, nil
	case err != nil:
		return nil, probe.Measured{}, err
	}
	performed.underProbe = mutated.record
	performed.durationMS = mutated.record.DurationMS
	performed.outputTail = mutated.record.OutputTail
	return performed, probe.Measured{
		Applied:     true,
		TimedOut:    mutated.record.TimedOut,
		Unstarted:   mutated.unstarted,
		ExitCode:    mutated.record.ExitCode,
		TestsRun:    mutated.record.TestsRun,
		TestsFailed: mutated.record.TestsFailed,
		Detail:      mutated.detail,
	}, nil
}

// gapRuns performs §5.2.6's baseline and then §5.4.2's placed run, all inside
// §5.6.1's lock.
//
// The placement, the run, and the removal are one call, because §5.4.2's
// removal "MUST happen even when the run fails or times out" and
// state.UnderSandboxTestFile is the only door onto the write that arranges it.
//
// §5.4.2 narrows the probe's own run to the directory cr placed the file in, as
// its only path, and §5.4.2's last clause is the profile that cannot take one: a
// profile with no `tests.paths_arg` has no way to name a path to its runner, so
// the run "MUST use the filter alone" and the runner's own discovery finds the
// placed file.
//
// The directory rather than the file is v0.4.1's correction, and it is measured.
// Handed the file, `go test` compiles it as the synthetic package
// `command-line-arguments`, which holds that one file and nothing the test
// refers to: measurement 4 part B's gap probe exited 1 on `undefined: Target`
// with no test run. A runner that builds a package out of the paths it is given
// cannot build one out of a test file alone, and a runner that discovers tests
// under a path is unharmed by being given the directory instead.
func gapRuns(
	setup *probeSetup, request *probeRequest, placement string,
) (*performedProbe, probe.GapMeasured, error) {
	var ownPaths []string
	if len(setup.tests.profile.Tests.PathsArg) > 0 {
		ownPaths = []string{probeRunPath(placement)}
	}
	performed, err := probeBaselines(setup, request, ownPaths)
	if err != nil {
		return nil, probe.GapMeasured{}, err
	}

	var placed *measuredRun
	err = setup.layout.UnderSandboxTestFile(
		request.owner, request.repo, request.pr, placement, request.test, func() error {
			ran, err := setup.tests.perform(request.filter, ownPaths)
			placed = ran
			return err
		})
	if err != nil {
		return nil, probe.GapMeasured{}, err
	}
	performed.underProbe = placed.record
	performed.durationMS = placed.record.DurationMS
	performed.outputTail = placed.record.OutputTail
	return performed, probe.GapMeasured{
		TimedOut:    placed.record.TimedOut,
		Unstarted:   placed.unstarted,
		ExitCode:    placed.record.ExitCode,
		TestsRun:    placed.record.TestsRun,
		TestsFailed: placed.record.TestsFailed,
		Detail:      placed.detail,
	}, nil
}

// baselineRun performs one of §5.2.2's baselines and stores §5.2.4's record for
// it, which is what probe.Ensure hands back to §5.5's `baseline` column.
func baselineRun(
	setup *probeSetup, request *probeRequest, spec probe.Spec,
) (stored run.Record, unclean string, err error) {
	ran, err := setup.tests.perform(spec.Filter, spec.Paths)
	if err != nil {
		return run.Record{}, "", err
	}
	// §5.1.6's check after this run too, for round 12's
	// baseline-contamination reason: a baseline whose sandbox drifted
	// under it is nobody's baseline, and §5.2.5's verdict is what says so.
	unclean, err = sandbox.Unclean(setup.src, setup.glob)
	if err != nil {
		return run.Record{}, "", err
	}
	ran.record.Contaminated = unclean != ""
	if _, err := recordRun(
		setup.layout, request.owner, request.repo, request.pr,
		setup.stamp, ran.record); err != nil {
		return run.Record{}, "", err
	}
	return *ran.record, unclean, nil
}

// recordProbe stores §5.5's probe record and, when a run happened, §5.2.4's
// record for the run it was measured by.
//
// Both writes happen under one hold of §2.3.1's lock, because the run record
// carries the probe's id: allocating the id in one hold and referencing it in
// another would let a second cr run allocate the same one in between.
func recordProbe(
	layout state.Layout, owner, repo string, pr int, at state.Stamp,
	underProbe *run.Record, record *probe.Record,
) (runID, probeID string, err error) {
	held, err := layout.LockPR(owner, repo, pr)
	if err != nil {
		return "", "", err
	}
	runID, probeID, err = appendProbe(layout, owner, repo, pr, held, at, underProbe, record)
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
//
// A record arriving with an id already on it is a gap probe's, whose id §5.4.2
// fixed before the placement. That id is checked against the allocation rather
// than trusted: §5.5.2 has a finding reference a probe by id, so two records
// sharing one would make the reference ambiguous, and the window is real
// because the reservation is read outside §2.3.1's lock.
func appendProbe(
	layout state.Layout, owner, repo string, pr int,
	held *state.Lock, at state.Stamp, underProbe *run.Record, record *probe.Record,
) (runID, probeID string, err error) {
	// §5.5's per-kind vocabulary, asked at the boundary where a record is
	// stored rather than where a ladder produced its value: what a reader
	// of probes.ndjson is promised is that every stored line carries a
	// result its kind admits, and only a check on the way in promises it.
	// Nothing has been written when this refuses — the run record is
	// appended below, so the probe's own run does not reach disk without
	// the probe it measured.
	if err := probe.CheckResult(record); err != nil {
		return "", "", err
	}
	stored, err := state.ReadRecords[probe.Record](layout, owner, repo, pr, state.FileProbes)
	if err != nil {
		return "", "", err
	}
	// §5.6.4 once more, at the write, where §2.3.1's lock makes the count
	// exact. §5.6.1's lock is named after the repository's path, so two cr
	// runs from two checkouts of one pull request hold different probe
	// locks and write the same probes.ndjson; this is the check neither
	// lock covers. Such a run has been performed, and its refusal says so.
	capped, err := roundCap(layout, owner, repo, stored, at.Round)
	if err != nil {
		return "", "", err
	}
	if capped.Reached() {
		return "", "", &probe.RoundCapReachedError{Cap: capped, Performed: true}
	}
	switch next := probe.NextID(stored); {
	case record.ID == "":
		record.ID = next
	case record.ID != next:
		return "", "", &probe.IDTakenError{Reserved: record.ID, Next: next}
	}
	if underProbe != nil {
		// §5.2.6's fence, set here and nowhere else: the probe's own
		// run carries the probe's id, so it can never be resolved as
		// the next probe's baseline.
		underProbe.Probe = record.ID
		if runID, err = appendRun(layout, owner, repo, pr, held, at, underProbe); err != nil {
			return "", "", err
		}
	}
	if err := state.AppendStamped(held, state.FileProbes, at, []*probe.Record{record}); err != nil {
		return "", "", err
	}
	return runID, record.ID, nil
}
