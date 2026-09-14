package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/run"
	"github.com/deligoez/cr/internal/state"
)

// cappedAt sets the fixture repository's `probe.max_per_round`, which every
// cr process pointed at the fixture's state resolves, spawned or in process.
func cappedAt(t *testing.T, l state.Layout, maxPerRound string) {
	t.Helper()
	require.NoError(t, os.WriteFile(l.RepoConfig(fixtureOwner, fixtureProject),
		[]byte(`{"probe": {"max_per_round": `+maxPerRound+`}}`), 0o600))
}

// concurrentProbe is one `cr probe run` process, with its two streams kept
// apart so the refused run's failure document is read without runner output
// in front of it.
type concurrentProbe struct {
	cmd            *exec.Cmd
	stdout, stderr bytes.Buffer
	done           chan struct{}
	err            error
}

func startConcurrentProbe(t *testing.T, prepared state.Layout, fixture, patch string) *concurrentProbe {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)
	p := &concurrentProbe{done: make(chan struct{})}
	p.cmd = exec.Command(self, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", patch)
	p.cmd.Dir = fixture
	p.cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"TMPDIR=" + os.TempDir(),
		state.HomeEnv + "=" + prepared.Root(),
		runAsCR + "=1",
	}
	p.cmd.Stdout, p.cmd.Stderr = &p.stdout, &p.stderr
	require.NoError(t, p.cmd.Start())
	go func() {
		p.err = p.cmd.Wait()
		close(p.done)
	}()
	t.Cleanup(func() {
		_ = p.cmd.Process.Kill()
		<-p.done
	})
	return p
}

// exitCode waits up to a minute for the process and returns its exit code.
func (p *concurrentProbe) exitCode(t *testing.T) int {
	t.Helper()
	select {
	case <-p.done:
	case <-time.After(60 * time.Second):
		require.FailNowf(t, "cr probe run did not finish within a minute",
			"stdout:\n%s\nstderr:\n%s", p.stdout.String(), p.stderr.String())
	}
	var exited *exec.ExitError
	if errors.As(p.err, &exited) {
		return exited.ExitCode()
	}
	require.NoError(t, p.err)
	return 0
}

// §5.6.4 under concurrency: two `cr probe run` processes started against a
// round one probe below its cap add exactly one probe record between them, and
// the other run is refused by the cap having run nothing.
//
// Audit round 14's list-31-4 measured the defect this pins: the cap was counted
// only before §5.6.1's lock, so both runs passed a count of one against a cap of
// two, both ran, and the round held three records.
//
// The test holds the probe lock while both processes start, so both take their
// pre-lock count while the round still holds one record and then wait on the
// lock with the sandbox untouched. That is the interleaving the defect needs,
// arranged rather than hoped for. Which process wins is not asserted; that one
// wins and one is refused is.
//
// "Ran nothing" is read off the runner's log. A refusal placed at the write
// rather than under the lock would leave the record count right while the
// refused run had performed its suite, and only the log shows that.
func TestConcurrentProbeRunsNeverPassTheRoundCap(t *testing.T) {
	prepared, fixture, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
	cappedAt(t, prepared, "2")
	patch := writePatch(t, fixtureDiff)
	require.NoError(t, runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", patch))
	require.Len(t, storedRecords(t, prepared, state.FileProbes), 1, "the round is one probe below its cap")
	before, err := os.ReadFile(log)
	require.NoError(t, err)

	root, err := filepath.EvalSymlinks(fixture)
	require.NoError(t, err)
	held, err := prepared.LockProbe(root, "qa", time.Second)
	require.NoError(t, err)
	probes := []*concurrentProbe{
		startConcurrentProbe(t, prepared, fixture, patch),
		startConcurrentProbe(t, prepared, fixture, patch),
	}
	// Long enough for both to have counted and to be waiting on the lock.
	time.Sleep(2 * time.Second)
	require.NoError(t, held.Unlock())

	codes := []int{probes[0].exitCode(t), probes[1].exitCode(t)}
	winner, refused := probes[0], probes[1]
	if codes[0] != 0 {
		winner, refused = probes[1], probes[0]
	}
	require.ElementsMatch(t, []int{0, ExitValidation}, codes,
		"one run succeeds and the other exits 1\nfirst:\n%s%s\nsecond:\n%s%s",
		probes[0].stdout.String(), probes[0].stderr.String(),
		probes[1].stdout.String(), probes[1].stderr.String())

	records := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, records, 2, "§5.6.4: exactly one probe record was added at a cap of two")
	assert.Equal(t, []any{"p1", "p2"}, []any{records[0]["id"], records[1]["id"]})

	filled := probe.RoundCap{Count: 2, Max: 2}
	reported := probeDocument(t, winner.stdout.String())
	assert.Equal(t, "p2", reported["probe"])
	assert.Equal(t, []any{filled.Disclosure()}, reported["honesty"],
		"the run that filled the round reports the count taken under the lock")

	spent := &probe.RoundCapReachedError{Cap: filled}
	failure := probeDocument(t, refused.stderr.String())
	assert.Equal(t, map[string]any{"error": spent.Error(), "hint": hintFor(spent)}, failure,
		"§5.6.4: the other run is refused by the cap, and before anything ran")
	assert.Empty(t, refused.stdout.String())

	after, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(after), "---\n")-strings.Count(string(before), "---\n"),
		"the runner started once between the two processes: the refused run performed no suite")
}

// §5.6.4 at the record's write: a probe record that would take the round past
// its cap is refused and nothing is stored, and the refusal says the run was
// performed rather than that nothing was run.
//
// Two cr runs from two checkouts of one pull request hold §5.6.1 locks named
// after different paths, so the count under the lock cannot see the other's
// record; the write, under §2.3.1's per-PR lock, is where the count is exact.
func TestAProbeRecordPastTheRoundCapIsRefusedAtItsWrite(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, gapProbeRunner, gapProbeTemplate)
	cappedAt(t, prepared, "1")
	at := state.Stamp{Head: "be7e2c7", Round: 2}

	_, id, err := recordProbe(prepared, fixtureOwner, fixtureProject, fixturePRNumber, at, nil,
		&probe.Record{Kind: probe.Gap, Result: "passed", Target: "app.go:3", Baseline: "r1"})
	require.NoError(t, err)
	require.Equal(t, "p1", id, "the round's one probe fits its cap")

	measured := &run.Record{Filter: "Retry", OutputTail: "Tests:  1 passed\n"}
	_, _, err = recordProbe(prepared, fixtureOwner, fixtureProject, fixturePRNumber, at, measured,
		&probe.Record{Kind: probe.Gap, Result: "failed", Target: "app.go:3", Baseline: "r1"})

	var refused *probe.RoundCapReachedError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, probe.RoundCapReachedError{Cap: probe.RoundCap{Count: 1, Max: 1}, Performed: true},
		*refused)
	assert.Equal(t, "§5.6.4: 1 of 1 probes run this round against probe.max_per_round, so the cap "+
		"is reached and no further probe runs in this round; raise probe.max_per_round to run more: "+
		"this run's record would be number 2, so it was not stored; raise probe.max_per_round, or "+
		"open a new round with cr brief when the head moves", err.Error())
	assert.Equal(t, ExitValidation, exitCodeFor(err))

	assert.Len(t, storedRecords(t, prepared, state.FileProbes), 1,
		"§5.6.4: the record past the cap never reached probes.ndjson")
	assert.Empty(t, storedRecords(t, prepared, state.FileRuns),
		"nor did the run it was measured by")

	_, id, err = recordProbe(prepared, fixtureOwner, fixtureProject, fixturePRNumber,
		state.Stamp{Head: "be7e2c7", Round: 3}, nil,
		&probe.Record{Kind: probe.Gap, Result: "passed", Target: "app.go:3", Baseline: "r1"})
	require.NoError(t, err, "§5.6.4 caps per round: the next round's first probe is stored")
	assert.Equal(t, "p2", id)
}
