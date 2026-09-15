package sandbox

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/deligoez/cr/internal/state"
)

// SetupError reports a `sandbox.setup` command that failed.
//
// §3.1.3 fixes the shape for an external command cr drives: a non-zero exit
// fails with exit code 3 and the command's stderr reaches the user. A setup
// command is such a command — the profile names it and cr has never heard of
// it — so it fails in that shape rather than in one of its own, and
// internal/cli maps this type onto that code.
type SetupError struct {
	// Args is the command as it ran, program included, so what is
	// reported can be pasted back into a shell.
	Args []string
	// Stderr is what the command wrote to standard error, trimmed of
	// surrounding whitespace. It is the only diagnostic there is when a
	// tool cr knows nothing about refuses.
	Stderr string
	// Err is the failure os/exec reported: a non-zero exit status, or a
	// command that could not be started at all.
	Err error
}

func (e *SetupError) Error() string {
	ran := "sandbox.setup " + strings.Join(e.Args, " ")
	if e.Stderr == "" {
		return fmt.Sprintf("%s: %v", ran, e.Err)
	}
	return fmt.Sprintf("%s: %v: %s", ran, e.Err, e.Stderr)
}

// Unwrap exposes the underlying exec failure, so a caller can tell a command
// that ran and refused from one that never started.
func (e *SetupError) Unwrap() error { return e.Err }

// RunError reports a test runner that could not be started at all.
//
// It is the failure that is not a test result. A runner that ran and exited
// non-zero has said something about the code — §5.2.4 records that exit code and
// §5.3.4's ladder reads it — while a runner that never started has said nothing,
// and §3.1.3 gives that the shape every external command cr drives fails in:
// exit code 3, with what the attempt reported.
type RunError struct {
	// Args is the command as it was to have run, program included.
	Args []string
	// Err is what os/exec reported.
	Err error
}

func (e *RunError) Error() string {
	return fmt.Sprintf("cannot run %s: %v", strings.Join(e.Args, " "), e.Err)
}

// Unwrap exposes the underlying failure.
func (e *RunError) Unwrap() error { return e.Err }

// Run runs the profile's test command inside the sandbox and returns the
// runner's own exit status and whether the run was killed for exceeding
// timeout (§5.2.1, §5.2.3).
//
// dir is the sandbox, and it is the whole point of the function. A suite run in
// the main checkout would be measuring the user's working tree — mid-edit,
// possibly on another branch — and reporting the answer as if it came from the
// pull request head, while §2.2 forbids cr to have put the probe there in the
// first place. So the directory is pinned here and comes from state.Layout by
// way of §5.1.6's check, never from a caller's own join.
//
// The output goes to log as it is produced rather than being returned.
// §5.2.1 also asks for the exit code, the duration and a bounded tail of the
// output to be *recorded*, and that record is §5.2.4's; what a person watching
// the command wants meanwhile is to see the suite run.
//
// A non-zero exit is returned as a value, not as an error. A failing suite is an
// ordinary and often intended outcome — §5.3.4's whole ladder is built on
// reading one — so the number is reported and its meaning is left to the section
// that owns it. So is the timeout: §5.2.3 has the run recorded as `timeout`,
// and §5.3.4 puts that on a rung of its own above `error`, because a suite that
// never finished said nothing about the code while one that failed did.
//
// A run that outlives timeout is killed by process *group*, which is what
// Setpgid buys. A test runner starts children — `composer`, `php`, a database
// client — and signalling the parent alone leaves them running: they hold the
// test database §5.6's lock exists to serialise access to, and they hold the
// pipe this function's output travels through, so cmd.Wait would go on
// blocking until they chose to exit. The child is therefore put in a group of
// its own and the negative pid signals every process in it. SIGKILL rather
// than SIGTERM: the run has already been given the whole budget the profile
// set, and a runner that ignores a request to stop would take a second budget
// to discover.
//
// A timeout of zero or less is no timeout at all rather than an immediate
// kill. §2.4 gives `tests.timeout_seconds` a default of 900 and refuses a
// non-positive value, so no loaded profile reaches here without a budget; a
// caller that passes none has declined to bound the run, and killing it before
// it started would fail every run instead.
//
// The environment is inherited whole, for the reason runSetup inherits it: the
// runner is a tool the user names and cr has never heard of, and an allowlist
// here would be a list of names cr cannot know.
//
// runner is the pull request's runner lock, which the runner inherits so a
// later run can find it if cr does not outlive it; nil starts a runner no later
// run looks for.
func Run(
	argv []string, dir string, log io.Writer, timeout time.Duration, runner *state.RunnerLock,
) (code int, timedOut bool, err error) {
	exit, err := RunExit(argv, dir, log, timeout, runner)
	return exit.Code, exit.TimedOut, err
}

// Exit is how one run of the test command ended.
type Exit struct {
	// Code is the runner's own exit status, negative for a process that
	// exited on a signal rather than by returning.
	Code int
	// TimedOut says the run was killed for exceeding its timeout.
	TimedOut bool
	// Signal names the signal a run that was not timed out exited on, and
	// is empty when it returned. §5.3.4's third rung and §5.4.3's second
	// answer such a run `error`, and the name is what tells a runner that
	// crashed from one that never started.
	Signal string
}

// InterruptedError reports a run cr stopped because cr itself was asked to
// stop, by SIGINT or SIGTERM.
//
// The runner is in a process group of its own, so a Ctrl+C in a terminal
// reaches cr and never the runner; cr ending without passing it on would leave
// the suite running unlocked against the sandbox and its test database. So the
// group is killed and reaped first, and the error then unwinds through the
// revert of §5.3.3 and the release of §5.6.1's lock like any other failure,
// recording nothing: a suite that was stopped said nothing about the code.
type InterruptedError struct {
	// Signal is the signal cr received, in the words os gives it.
	Signal string
}

func (e *InterruptedError) Error() string {
	return fmt.Sprintf("cr received %s while the test runner ran: the runner's process group was killed "+
		"and nothing was recorded for the run", e.Signal)
}

// BeforeRunnerRelease, when set, is called with the sandbox between the start
// of a held runner and its release. Only tests set it: it is where a kill of cr
// inside that window is placed.
var BeforeRunnerRelease func(dir string)

// heldRunner is the name cr's own binary is started under to hold a test runner
// until the runner's process group is recorded, as state.RunnerHold describes,
// and heldRunnerAbandoned the status such a process exits with when it runs
// nothing.
const (
	heldRunner          = "cr-held-runner"
	heldRunnerAbandoned = 125
)

// init makes a process started under heldRunner the runner it holds, before
// any other work of cr's begins.
//
// It is an init rather than a line of main so that every binary that starts a
// runner through this package serves the hold: cr, and the test binaries that
// start one without being cr. os.Args[1] is the program exec.Command resolved in
// cr and the rest is the runner's argv as cr would have started it, so exec
// replaces the process with exactly the runner an unheld start ran — in the
// same directory, process group and environment, with the runner lock at the
// same descriptor.
func init() {
	if len(os.Args) < 3 || os.Args[0] != heldRunner {
		return
	}
	if state.AwaitRunnerRelease() {
		state.ReportRunnerExecFailure(syscall.Exec(os.Args[1], os.Args[2:], os.Environ()))
	}
	os.Exit(heldRunnerAbandoned)
}

// RunExit is Run, reporting the signal a run exited on beside its status.
//
// A runner started with a runner lock is held: cr starts its own binary in the
// runner's place, records the group, and only then lets it become the runner.
// A cr killed in between leaves a process that exits running nothing, rather
// than a runner whose group no later run can find.
func RunExit(
	argv []string, dir string, log io.Writer, timeout time.Duration, runner *state.RunnerLock,
) (Exit, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	program := cmd.Path
	hold, err := holdRunner(cmd, runner)
	if err != nil {
		return Exit{}, &RunError{Args: slices.Clone(argv), Err: err}
	}
	// Installed before the start, so no moment exists at which a runner
	// is alive and a stop request would end cr without reaching it.
	stopping := make(chan os.Signal, 1)
	signal.Notify(stopping, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopping)
	if err := cmd.Start(); err != nil {
		if hold != nil {
			hold.Abandon()
		}
		return Exit{}, &RunError{Args: slices.Clone(argv), Err: err}
	}
	// Read before the waiting goroutine exists, so the pid the kill uses is
	// never read beside a concurrent Wait.
	group := cmd.Process.Pid
	if hold != nil {
		if err := releaseRunner(cmd, hold, runner, dir, program); err != nil {
			return Exit{}, err
		}
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()

	var expired <-chan time.Time
	if timeout > 0 {
		budget := time.NewTimer(timeout)
		defer budget.Stop()
		expired = budget.C
	}
	select {
	case err := <-finished:
		var exit *exec.ExitError
		switch {
		case err == nil:
			return Exit{}, nil
		case errors.As(err, &exit):
			return Exit{Code: exit.ExitCode(), Signal: signalOf(exit)}, nil
		}
		return Exit{}, &RunError{Args: slices.Clone(argv), Err: err}
	case <-expired:
		// The error is discarded because there is nothing left to do
		// about it: a group that has already exited answers ESRCH,
		// and the run is being reported as a timeout either way.
		_ = syscall.Kill(-group, syscall.SIGKILL)
		// Reaped before returning. The killed group is what closes the
		// output pipe, so this waits for the copy to finish rather
		// than on a grandchild that outlived its parent.
		var exit *exec.ExitError
		if errors.As(<-finished, &exit) {
			return Exit{Code: exit.ExitCode(), TimedOut: true}, nil
		}
		return Exit{TimedOut: true}, nil
	case received := <-stopping:
		_ = syscall.Kill(-group, syscall.SIGKILL)
		<-finished
		return Exit{Code: -1}, &InterruptedError{Signal: received.String()}
	}
}

// holdRunner turns cmd into the start of a held runner: cr's own binary under
// heldRunner, given the program cmd would have run and the runner's argv, with
// the runner lock and the hold's pipes as its descriptors from 3. Without a
// runner lock there is no group to record, and cmd is left to start the runner
// itself.
//
// A program exec.Command could not resolve is refused here with the error Start
// would have returned, because the held process would otherwise be started for
// a runner that cannot run.
func holdRunner(cmd *exec.Cmd, runner *state.RunnerLock) (*state.RunnerHold, error) {
	if runner == nil {
		return nil, nil
	}
	if cmd.Err != nil {
		return nil, cmd.Err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	hold, err := runner.Hold()
	if err != nil {
		return nil, err
	}
	cmd.Args = append([]string{heldRunner, cmd.Path}, cmd.Args...)
	cmd.Path = self
	cmd.ExtraFiles = hold.Inherited()
	return hold, nil
}

// releaseRunner records the process group of a started held runner and then
// releases it, returning a RunError when exec could not replace the held
// process with program.
//
// A record that fails abandons the hold, so the held process exits running
// nothing; the group is killed as well, for a process that has read the release
// some other way.
func releaseRunner(cmd *exec.Cmd, hold *state.RunnerHold, runner *state.RunnerLock, dir, program string) error {
	hold.Started()
	if BeforeRunnerRelease != nil {
		BeforeRunnerRelease(dir)
	}
	group := cmd.Process.Pid
	if err := runner.Started(group); err != nil {
		hold.Abandon()
		_ = syscall.Kill(-group, syscall.SIGKILL)
		return errors.Join(err, cmd.Wait())
	}
	if err := hold.Release(program); err != nil {
		_ = syscall.Kill(-group, syscall.SIGKILL)
		_ = cmd.Wait()
		return &RunError{Args: slices.Clone(cmd.Args[2:]), Err: err}
	}
	return nil
}

// StoppedRunner is a runner an earlier cr run left alive in the sandbox, which
// this run killed before doing anything in it.
type StoppedRunner struct {
	// Group is the process group that was killed.
	Group int
	// Lingering says a process that run started still held the runner
	// lock after the group was killed: one that left the group, which the
	// kill could not reach.
	Lingering bool
}

// Disclosure is the notice a run that stopped a runner prints, whatever the
// flags say: a suite a previous cr left running shared the sandbox and its
// test database with nothing holding §5.6.1's lock.
func (s *StoppedRunner) Disclosure() string {
	notice := fmt.Sprintf("a test runner an earlier cr run left running in process group %d was killed "+
		"before this run, per §5.3.3", s.Group)
	if s.Lingering {
		notice += "; a process it started outside that group is still running"
	}
	return notice
}

// leftRunnerGrace is how long a killed runner group is given to exit before
// the run goes ahead, and leftRunnerPoll how often it is asked. A SIGKILL is
// not refused, so the grace bounds only a process that left the group.
const (
	leftRunnerGrace = 10 * time.Second
	leftRunnerPoll  = 20 * time.Millisecond
)

// stopLeftRunner kills the runner an earlier cr run left alive in the pull
// request's sandbox, and returns nil when there is none.
//
// It is the half of §5.3.3's killed run that §5.1.6's recreation does not
// cover. Recreating the sandbox reverts the mutation, but the runner that was
// measuring it keeps running with nothing holding §5.6.1's lock, underneath
// the recreation and beside the next run's own suite. So it is killed first,
// and waited for until every process holding the runner lock has exited.
func stopLeftRunner(src *Sources) (*StoppedRunner, error) {
	left, err := src.Layout.LeftRunner(src.Owner, src.Repo, src.PR)
	if err != nil || left == nil {
		return nil, err
	}
	_ = syscall.Kill(-left.Group, syscall.SIGKILL)
	deadline := time.Now().Add(leftRunnerGrace)
	gone, err := left.Gone()
	for err == nil && !gone && time.Now().Before(deadline) {
		time.Sleep(leftRunnerPoll)
		gone, err = left.Gone()
	}
	if err := errors.Join(err, left.Release()); err != nil {
		return nil, err
	}
	return &StoppedRunner{Group: left.Group, Lingering: !gone}, nil
}

// signalOf names the signal a finished process exited on, in the words os
// gives it, and is empty for one that returned.
//
// It reads the process state's own description rather than its wait status:
// this file may name only the process-control identifiers of `syscall`, and
// os spells a signalled exit as `signal: <name>` for every such process.
//
// os appends ` (core dumped)` where the kernel wrote a core, which Linux does
// for a segmentation fault and macOS by default does not. The suffix is
// dropped so one run names its signal the same way on every platform; CI on
// Linux measured `segmentation fault (core dumped)` where macOS gave
// `segmentation fault`.
func signalOf(exit *exec.ExitError) string {
	name, signalled := strings.CutPrefix(exit.String(), "signal: ")
	if !signalled {
		return ""
	}
	return strings.TrimSuffix(name, " (core dumped)")
}

// setupArgv splits one `sandbox.setup` entry into the argv it runs as.
//
// There is no shell. §3.1.1 already settled the same question for the tracker
// command — an argv array, substituted per element, so nothing quotes and
// nothing splits — and a shell here would buy a run whose result depends on
// which shell is installed and on what the ambient environment expands to,
// which §2.1.1 asks cr not to have. A profile that needs a shell writes one:
// `sh /path/to/setup.sh` is a command like any other.
//
// An entry that splits to nothing is not a command, and is reported by the
// caller rather than skipped.
func setupArgv(command string) []string {
	return strings.Fields(command)
}

// runSetup runs one setup command in dir, which is §5.1.3's sandbox root.
//
// The environment is inherited whole, for the reason internal/intent inherits
// it: the command is a tool the user names and cr has never heard of —
// `composer`, `npm`, `make` — and an allowlist here would be a list of names cr
// cannot know, the first casualty being whatever credential or cache path the
// tool needs to work at all. What cr does pin is the directory, because §5.1.3
// says where the command runs.
//
// Standard output is discarded and standard error kept. §5.1.3 asks for the
// commands to run, not for their output to be recorded — that is §5.2.4's
// business, for runs that evidence a finding — but a command that failed has
// to be able to say why.
func runSetup(argv []string, dir string) error {
	var stderr bytes.Buffer
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return &SetupError{
			Args:   slices.Clone(argv),
			Stderr: strings.TrimSpace(stderr.String()),
			Err:    err,
		}
	}
	return nil
}
