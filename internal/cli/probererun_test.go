package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/state"
)

// earlierHead is the head a seeded probe was recorded at: the one the round
// that posted the record stood at before the author pushed.
const earlierHead = "0ld0000"

// seedProbes writes probes.ndjson for one pull request, one line per record, as
// cr's own writer would have left it.
func seedProbes(t *testing.T, path string, records ...probe.Record) {
	t.Helper()
	lines := make([]string, 0, len(records))
	for i := range records {
		line, err := json.Marshal(&records[i])
		require.NoError(t, err)
		lines = append(lines, string(line))
	}
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
}

// seededMutation is p1 as a round before this one recorded it: §5.3.1's
// mutation of the fixture, at an earlier head.
func seededMutation() probe.Record {
	return probe.Record{
		ID: "p1", Kind: probe.Mutation, Stamp: state.Stamp{Head: earlierHead, Round: 1},
		Input: fixtureDiff, Result: "test-failed", Baseline: "r1", Target: "app.go:3",
		OutputTail: "Tests:  1 failed\n",
	}
}

// §5.5.4: `cr probe run --rerun <probe-id>` performs the stored mutation at
// the current round's head, as if its patch had been given with `--patch`, and
// writes a new record whose `rerun_of` names the probe it re-ran.
//
// The stored probe is a round-1 record at an earlier head, which is the case
// the flag exists for: a posted record's probe re-run after the author pushed.
// The runner's log is what says the stored patch, and not something else, was
// what the suite ran against.
func TestAReRunPerformsTheStoredMutationAtTheRoundsHead(t *testing.T) {
	prepared, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
	seedProbes(t, prepared.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileProbes),
		seededMutation())

	reported := probeDocument(t, throughAPipe(t,
		"probe", "run", fixturePR, "--repo", fixtureSlug, "--rerun", "p1"))

	assert.Equal(t, "p2", reported["probe"])
	assert.Equal(t, "p1", reported["rerun_of"])
	assert.Equal(t, "mutation", reported["kind"])
	assert.Equal(t, "no-test-failed", reported["result"])

	observed, err := os.ReadFile(log)
	require.NoError(t, err, "the profile's test command never ran")
	assert.Equal(t, fixtureSource+"---\n"+fixtureMutated+"---\n", string(observed),
		"§5.5.4: the baseline and then the stored patch, at the round's head")

	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 2, "a re-run is a new record and the one it re-ran is kept")
	assert.Equal(t, earlierHead, probes[0]["head"], "the stored probe is left as it was")
	assert.NotContains(t, probes[0], "rerun_of")
	rerun := probes[1]
	assert.Equal(t, "p2", rerun["id"])
	assert.Equal(t, "p1", rerun["rerun_of"])
	assert.Equal(t, float64(2), rerun["round"], "the re-run is the current round's")
	assert.NotEqual(t, earlierHead, rerun["head"], "the re-run is stamped with the round's head")
	assert.Equal(t, fixtureDiff, rerun["input"])
	assert.Equal(t, "app.go:3", rerun["target"], "§5.3.2 derives the target from the patch again")
}

// §5.5.4: a gap probe's re-run places the stored test and addresses the stored
// target, which nothing on the command line supplies.
func TestAGapReRunUsesTheStoredTestAndTarget(t *testing.T) {
	const rerunPlacement = "tests/cr_probe_p2.txt"
	prepared, _, _, _ := probeFixture(t,
		"cat "+rerunPlacement+" 2>/dev/null || echo 'the probe file is not there'\n"+
			"echo 'Tests:  5 passed'\n",
		gapProbeTemplate)
	seedProbes(t, prepared.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileProbes),
		probe.Record{
			ID: "p1", Kind: probe.Gap, Stamp: state.Stamp{Head: earlierHead, Round: 1},
			Input: gapProbeTest, Result: "failed", Baseline: "r1", Target: "app.go:3",
		})

	reported := probeDocument(t, throughAPipe(t,
		"probe", "run", fixturePR, "--repo", fixtureSlug, "--rerun", "p1"))

	assert.Equal(t, "p2", reported["probe"])
	assert.Equal(t, "p1", reported["rerun_of"])
	assert.Equal(t, "gap", reported["kind"])
	assert.Equal(t, "app.go:3", reported["target"], "§5.5.4 carries a gap probe's target over")
	assert.Equal(t, "passed", reported["result"])

	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 2)
	assert.Equal(t, "p1", probes[1]["rerun_of"])
	assert.Equal(t, gapProbeTest, probes[1]["input"])
	assert.Equal(t, "app.go:3", probes[1]["target"])
	assert.Contains(t, probes[1]["output_tail"], gapProbeTest,
		"the stored test was placed where the new id puts it, and the suite read it")
}

// §5.5.4: every input flag beside `--rerun` aborts with exit code 2, before
// anything runs. The stored probe carries every input, so a flag beside it
// would run an experiment other than the one `rerun_of` would name.
func TestEveryInputFlagBesideAReRunIsRefused(t *testing.T) {
	patch := writePatch(t, fixtureDiff)
	supplied := writeProbeTest(t)

	for flag, value := range map[string]string{
		"--proposal": "x1",
		"--kind":     "mutation",
		"--patch":    patch,
		"--test":     supplied,
		"--target":   "app.go:3",
		"--filter":   "Retry",
		"--path":     "app.go",
	} {
		t.Run(flag, func(t *testing.T) {
			prepared, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
			seedProbes(t, prepared.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileProbes),
				seededMutation())

			err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
				"--rerun", "p1", flag, value)

			require.Error(t, err)
			assert.Contains(t, err.Error(), flag+" is rejected beside --rerun")
			assert.Contains(t, err.Error(), "§5.5.4")
			assert.Equal(t, ExitUsage, exitCodeFor(err))
			assert.NoFileExists(t, log, "the refusal comes before any suite is run")
			assert.Len(t, storedRecords(t, prepared, state.FileProbes), 1, "nothing was recorded")
		})
	}
}

// §5.5.4: an id naming no probe of the pull request is refused with exit code
// 1, and nothing runs.
func TestAReRunOfAProbeThePullRequestDoesNotHoldIsRefused(t *testing.T) {
	prepared, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
	seedProbes(t, prepared.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileProbes),
		seededMutation())

	err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug, "--rerun", "p9")

	require.Error(t, err)
	var unknown *UnknownProbeError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, "p9", unknown.ID)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.NoFileExists(t, log, "the refusal comes before any suite is run")
}

// §5.5.4 and §9.5.4 together, across the push that makes a re-run worth
// having: a posted record names a probe of round 1, `cr probe run --rerun`
// repeats it in round 2, and `cr recheck` reports that re-run and its result —
// without starting the suite itself, and without the re-run having touched the
// record's `probe` or grade.
func TestRecheckReportsTheReRunAndRunsNothing(t *testing.T) {
	prepared, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
	seedProbes(t, prepared.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileProbes),
		seededMutation())
	posted := strings.Replace(settledRecord("f3", "finding", "posted", "PRRT_a"),
		`"summary"`, `"probe":"p1","grade":"probed","summary"`, 1)
	holdRecords(t, prepared, fixtureOwner, fixtureProject, fixturePRNumber, posted)
	findings := prepared.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	before, err := os.ReadFile(findings)
	require.NoError(t, err)

	probeDocument(t, throughAPipe(t, "probe", "run", fixturePR, "--repo", fixtureSlug, "--rerun", "p1"))

	after, err := os.ReadFile(findings)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "§5.5.4: a re-run changes no record's probe or grade")
	require.NoError(t, os.Remove(log))

	threadsAnswering(t)
	printed, err := runCLIPrinting(t, "recheck", fixturePR, "--repo", fixtureSlug)

	require.NoError(t, err)
	assert.NoFileExists(t, log, "§9.5.4: `cr recheck` MUST NOT run a probe")
	var report struct {
		Concerns []struct {
			ID    string         `json:"id"`
			Rerun map[string]any `json:"rerun"`
		} `json:"concerns"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	require.Len(t, report.Concerns, 1)
	rerun := report.Concerns[0].Rerun
	head := storedRecords(t, prepared, state.FileProbes)[1]["head"]
	assert.Equal(t, map[string]any{
		"of": "p1", "head": head, "ran": true, "probe": "p2", "result": "no-test-failed",
		"command": "cr probe run " + fixturePR + " --repo " + fixtureSlug + " --rerun p1",
	}, rerun)
}
