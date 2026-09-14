package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// §5.3.2 has the evidence chain run "on the patch cr executed", and §5.5 stores
// the patch whole as the record's `input`, which `cr draft` shows a colleague.
//
// QA found a patch whose second file section was a rename — `rename to
// ../../moved.php` — accepted: the rename was never executed, and the stored
// input still showed it. A patch line cr would not execute is now refused with
// §11.2's 1, naming the line, before any suite runs and before anything is
// recorded. The `diff --git` and `index` lines git writes stay accepted, which
// the first case also holds.
func TestAPatchLineCrWouldNotExecuteIsRefused(t *testing.T) {
	const hunk = "diff --git a/app.go b/app.go\nindex 1a2b3c4..5d6e7f8 100644\n" + fixtureDiff
	cases := map[string]struct {
		patch   string
		line    int
		problem string
	}{
		"a rename": {
			patch: hunk + "diff --git a/app.go b/../../moved.go\nsimilarity index 100%\n" +
				"rename from app.go\nrename to ../../moved.go\n",
			line: 11,
			problem: `"similarity index 100%" is a rename, copy or mode header, which cr does not execute: ` +
				"a probe only applies hunks to files the sandbox holds, so the probe record's input " +
				"would show a step that never ran",
		},
		"a mode change": {
			patch: "diff --git a/app.go b/app.go\nold mode 100644\nnew mode 100755\n" + fixtureDiff,
			line:  2,
			problem: `"old mode 100644" is a rename, copy or mode header, which cr does not execute: ` +
				"a probe only applies hunks to files the sandbox holds, so the probe record's input " +
				"would show a step that never ran",
		},
		"a line that is no header": {
			patch:   fixtureDiff + "applied by hand afterwards\n",
			line:    8,
			problem: `"applied by hand afterwards" is neither a file header nor a hunk line, so cr would not execute it`,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			prepared, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
			patch := writePatch(t, c.patch)

			err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
				"--kind", "mutation", "--patch", patch)
			var malformed *git.MalformedPatchError
			require.True(t, errors.As(err, &malformed), "refused as a patch cr cannot read: %v", err)
			assert.Equal(t, fmt.Sprintf("%s line %d: %s", patch, c.line, c.problem), err.Error())
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.NoFileExists(t, log, "the refusal comes before any suite is run")
			assert.Empty(t, storedRecords(t, prepared, state.FileProbes), "nothing is recorded")
		})
	}
}

// §5.3.4's first rung through the command, with its reason: a patch that does
// not apply records `error`, and the record and the output both say which file
// and line refused and what the patch expected beside what the file holds.
//
// QA found the record carrying nothing but `error`, with an empty output tail
// and no honesty line, so a stale patch could not be told from a dead runner.
func TestAPatchThatDoesNotApplyRecordsWhatDidNotMatch(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	patch := writePatch(t, "--- a/app.go\n+++ b/app.go\n@@ -1,3 +1,3 @@\n package app\n \n"+
		"-func Retry() { rewritten() }\n+func Retry() {}\n")
	const reason = "§5.3.4's first rung: the mutation patch did not apply cleanly, so no tests were run: " +
		`app.go line 3: the hunk does not match the file: the patch expected "func Retry() { rewritten() }" ` +
		`and the file holds "func Retry() { backoff() }"`

	shown := runProbe(t, patch)
	assert.Equal(t, "error", shown["result"])
	assert.Equal(t, reason, shown["reason"])

	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 1)
	assert.Equal(t, "error", probes[0]["result"])
	assert.Equal(t, reason, probes[0]["reason"])
}

// §5.3.4's third rung and §5.4.3's second, with their reasons: a runner that
// could not be started says so, with what the attempt reported, in the record
// and in the output of both kinds of probe.
func TestAProbeWhoseRunnerCouldNotStartRecordsWhy(t *testing.T) {
	for _, kind := range []struct {
		name, rung string
		run        func(t *testing.T) map[string]any
	}{
		{"mutation", "§5.3.4's third rung", func(t *testing.T) map[string]any {
			return runProbe(t, writePatch(t, fixtureDiff))
		}},
		{"gap", "§5.4.3's second rung", func(t *testing.T) map[string]any {
			return runGap(t, writeProbeTest(t))
		}},
	} {
		t.Run(kind.name, func(t *testing.T) {
			prepared, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n", gapProbeTemplate)
			runner := filepath.Join(filepath.Dir(log), "runner.sh")
			require.NoError(t, os.Remove(runner))
			reason := kind.rung + ": the runner could not be started: cannot run " + runner +
				": fork/exec " + runner + ": no such file or directory"

			shown := kind.run(t)
			assert.Equal(t, "error", shown["result"])
			assert.Equal(t, reason, shown["reason"])
			probes := storedRecords(t, prepared, state.FileProbes)
			require.Len(t, probes, 1)
			assert.Equal(t, reason, probes[0]["reason"])
		})
	}
}

// The same two rungs for a runner that started and exited on a signal: the
// reason names the signal, so a crashing runner is told apart from a missing
// one.
func TestAProbeWhoseRunnerExitedOnASignalRecordsWhichSignal(t *testing.T) {
	for _, kind := range []struct {
		name, reason, runner string
		run                  func(t *testing.T) map[string]any
	}{
		{
			name:   "mutation",
			reason: "§5.3.4's third rung: the runner exited on a signal (killed)",
			runner: onlyWhenMutated("  echo 'Tests:  4 passed'\n  kill -KILL $$\n"),
			run: func(t *testing.T) map[string]any {
				return runProbe(t, writePatch(t, fixtureDiff))
			},
		},
		{
			name:   "gap",
			reason: "§5.4.3's second rung: the runner exited on a signal (segmentation fault)",
			runner: onlyWithTheProbeFile("  echo 'Tests:  5 failed'\n  kill -SEGV $$\n"),
			run: func(t *testing.T) map[string]any {
				return runGap(t, writeProbeTest(t))
			},
		},
	} {
		t.Run(kind.name, func(t *testing.T) {
			prepared, _, _, _ := probeFixture(t, kind.runner, gapProbeTemplate)

			shown := kind.run(t)
			assert.Equal(t, "error", shown["result"])
			assert.Equal(t, kind.reason, shown["reason"])
			probes := storedRecords(t, prepared, state.FileProbes)
			require.Len(t, probes, 1)
			assert.Equal(t, kind.reason, probes[0]["reason"])
		})
	}
}

// The terminal rendering prints the reason beneath the result, exactly as the
// record carries it, and prints no reason line for a result that has none.
func TestTheProbeRenderingPrintsTheReasonForAnError(t *testing.T) {
	render := func(t *testing.T, reason string) string {
		t.Helper()
		var printed bytes.Buffer
		out := &writer{out: &printed, mode: ModeText}
		require.NoError(t, out.emit(&probeRunResult{
			Probe: "p1", Kind: "mutation", Sandbox: "/tmp/sandbox",
			Command: []string{"pest"}, Result: "error", Reason: reason, Target: "app.go:3",
			Baseline: "r1", Establishes: establishesNothing,
			Warnings: []string{}, Honesty: []string{},
		}))
		return printed.String()
	}

	const reason = "§5.3.4's third rung: the runner exited on a signal (killed)"
	assert.Contains(t, render(t, reason), "  result   error\n  reason   "+reason+"\n  evidence ")
	assert.Contains(t, render(t, ""), "  result   error\n  evidence ")
}
