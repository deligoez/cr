package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// qualityGate returns the gate command tp runs at `tp done`.
func qualityGate(t *testing.T) string {
	t.Helper()

	var file struct {
		Workflow struct {
			QualityGate string `json:"quality_gate"`
		} `json:"workflow"`
	}
	require.NoError(t, json.Unmarshal(repoFile(t, "spec/0.1.0.tasks.json"), &file))
	require.NotEmpty(t, file.Workflow.QualityGate, "the task file no longer declares a quality gate")
	return file.Workflow.QualityGate
}

// The quality gate is tp configuration, not code, so nothing in the compiler or
// the linter notices when a step is dropped from it. A gate step that can
// disappear without a test failing is exactly the blindness the step was added
// to close, so each one is named here.
//
// deadcode is the reason this test exists: golangci-lint's `unused` skips
// exported identifiers by design, so an exported function with zero callers
// passes `golangci-lint run`. Only the call graph notices, and only if the gate
// still runs it.
func TestQualityGateRunsEveryStep(t *testing.T) {
	gate := qualityGate(t)
	for _, step := range []string{
		"go test -race ./...",
		"golangci-lint run",
		"./scripts/deadcode.sh",
	} {
		assert.Contains(t, gate, step, "quality gate no longer runs this step")
	}
}

// A gate is only as strong as the weakest place it runs. `tp done` runs it on a
// developer's machine, but nothing made the two automated paths agree: the CI
// workflow ran `go test ./...` and golangci-lint — no -race, no deadcode — and
// the GoReleaser before-hooks ran `go test ./...` alone, so a change that failed
// the gate locally passed on the pull request that carried it and could be cut
// into a release. That is the same blindness the deadcode step was added to
// close, in two more places.
//
// The steps are derived from the gate string rather than restated, so a future
// gate change fails here until all three places carry it.
func TestEveryGateStepRunsInCIAndAtRelease(t *testing.T) {
	steps := strings.Split(qualityGate(t), "&&")
	require.NotEmpty(t, steps)

	ci := string(repoFile(t, ".github/workflows/ci.yml"))
	hooks := strings.Join(readGoreleaser(t).Before.Hooks, "\n")

	for _, step := range steps {
		step = strings.TrimSpace(step)
		assert.Contains(t, ci, step, "the CI workflow does not run this gate step")
		assert.Contains(t, hooks, step, "the GoReleaser before-hooks do not run this gate step")
	}
}

// A gate tool installed with `@latest` makes the gate a function of the tool
// rather than of the code: a new minor can enable a linter, change a default,
// or drop support for something, and CI turns red with nothing in this
// repository having changed. Both workflows therefore pin every tool they
// install, and this test is what keeps a later edit from quietly reverting to
// `@latest` -- the failure it would otherwise cause is a red CI run on a pull
// request that touched none of it.
func TestEveryGateToolIsPinned(t *testing.T) {
	for _, workflow := range []string{".github/workflows/ci.yml", ".github/workflows/release.yml"} {
		for line := range strings.SplitSeq(string(repoFile(t, workflow)), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "go install ") {
				continue
			}
			assert.NotContains(t, line, "@latest", "%s installs a gate tool unpinned: %s", workflow, line)
			assert.Regexp(t, `@v\d+\.\d+\.\d+$`, line, "%s installs a gate tool without an exact version: %s", workflow, line)
		}
	}
}

// pinReview is the date by which the pins above are to be looked at again. It
// is duplicated in both workflows as prose; this is the copy that fails.
const pinReview = "2026-11-30"

// A pin is the version that worked on a date, not a version that is right, and
// the failure mode is not the pin going stale -- it is nobody noticing. A
// review condition written as a comment is a condition that never gets
// checked: golangci-lint v2.13.0 shipped Go 1.27 support twelve days before
// the pin here was still v2.12.2, and the comment saying when to revisit would
// not have said a word about it.
//
// So the condition is a test. When this fails, the work is to install the
// current tools, run the gate against this tree, update the pins to what was
// measured, and move this date -- not to move the date alone.
func TestThePinsHaveBeenReviewedRecently(t *testing.T) {
	due, err := time.Parse(time.DateOnly, pinReview)
	require.NoError(t, err)

	assert.Falsef(t, time.Now().After(due),
		"the gate tool pins were last reviewed before %s: install the current golangci-lint and "+
			"deadcode, run the gate, pin what you measured, and move pinReview and the note in "+
			"both workflows", pinReview)

	for _, workflow := range []string{".github/workflows/ci.yml", ".github/workflows/release.yml"} {
		assert.Contains(t, string(repoFile(t, workflow)), pinReview,
			"%s no longer names the review date the pins are held to", workflow)
	}
}

// Every shell script under scripts/ is executable, because the gate and the
// mutation run invoke them by path. An editor that rewrites a file can drop
// the bit without a diff a reviewer reads: measured 2026-09-23, an edit of
// scripts/mutation-run.sh turned 100755 into 100644, shipped in v0.11.0, and
// the next run exited 126 before doing anything. The same loss on deadcode.sh
// would break the gate itself.
func TestEveryShellScriptIsExecutable(t *testing.T) {
	scripts, err := filepath.Glob(filepath.Join("..", "..", "scripts", "*.sh"))
	require.NoError(t, err)
	require.NotEmpty(t, scripts, "scripts/ holds the gate's deadcode.sh")
	for _, script := range scripts {
		info, err := os.Stat(script)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode().Perm()&0o111, "%s is not executable", filepath.Base(script))
	}
}
