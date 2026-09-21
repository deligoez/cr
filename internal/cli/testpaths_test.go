package cli

import (
	"encoding/json"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// pathsArgProfile is §2.4's `tests.paths_arg` for probeFixture's runner: one
// flag and the path after it, so the argv a test asserts shows both the
// substitution and the repetition.
const pathsArgProfile = `"paths_arg":["--dir","{path}"]`

// passingRunner prints the recap probeFixture's patterns read and nothing else.
const passingRunner = "echo 'Tests:  4 passed'\n"

// document decodes a command's standard output as the one §12.1 document.
func document(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &decoded), "standard output is one document: %q", stdout)
	return decoded
}

// §5.2.1 and §2.4 through the command: every `--path` reaches the runner
// through `tests.paths_arg`, once per path and in the order given, and the run
// record §5.2.4 stores carries the paths beside the filter.
//
// The argv is asserted rather than the fact that a flag was accepted, because
// what §5.2.2 keys a baseline by is the population that ran — and a path cr
// recorded but did not pass to the runner would key a baseline by a narrowing
// that never happened.
func TestCrTestPassesEveryPathThroughTheProfilesPathsArg(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, passingRunner, pathsArgProfile)

	stdout, _ := streams(t, "test", fixturePR, "--repo", fixtureSlug,
		"--path", "tests/Unit", "--path", "tests/Feature")

	reported := document(t, stdout)
	command, ok := reported["command"].([]any)
	require.True(t, ok, "the document names the argv as it ran: %q", stdout)
	require.Len(t, command, 5, "the runner, then two arguments per path")
	assert.Equal(t, []any{"--dir", "tests/Unit", "--dir", "tests/Feature"}, command[1:],
		"§2.4: the argv is appended once per --path, with {path} replaced")
	assert.Equal(t, []any{"tests/Unit", "tests/Feature"}, reported["paths"])

	runs := storedRecords(t, prepared, state.FileRuns)
	require.Len(t, runs, 1)
	assert.Equal(t, []any{"tests/Unit", "tests/Feature"}, runs[0]["paths"],
		"§5.2.4: the run record carries the paths when any were given")
	assert.NotContains(t, runs[0], "filter", "and no filter row, because none was given")
}

// §2.4: "when absent, `--path` MUST abort with exit code 3 naming the profile."
//
// Dropping the path instead would run the whole suite and record it as the run
// the caller asked for, which §5.2.2 would then resolve as the baseline of a
// probe narrowed to one directory.
func TestCrTestRefusesAPathTheProfileCannotCarry(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, passingRunner)

	err := runCLI(t, "test", fixturePR, "--repo", fixtureSlug, "--path", "tests/Unit")

	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err), "§2.4: a --path with no tests.paths_arg exits 3")
	assert.Contains(t, err.Error(), "tests.paths_arg")
	assert.Contains(t, err.Error(), prepared.Profile("qa"), "§2.4: the refusal names the profile")
	assert.NoDirExists(t, prepared.Sandbox(fixtureOwner, fixtureProject, fixturePRNumber),
		"a refused run builds no sandbox and executes nothing")
}

// §5.2.1: "Every `--path` MUST be relative, clean, and resolve inside the
// sandbox, or the command aborts with exit code 2; `cr` MUST NOT check that it
// exists."
//
// Both halves are driven through the command, on both commands that take the
// flag. The last case is the one that would be easy to get wrong in the safe-
// looking direction: a path nothing exists at is admitted, because the runner —
// not cr — decides what a path means, and §5.1.3's setup may not have created
// it yet at the moment the flag is parsed.
func TestBothRunningCommandsRefuseAPathThatIsNotRelativeCleanAndInside(t *testing.T) {
	for name, tc := range map[string]struct {
		path    string
		refused string
	}{
		"an absolute path":           {"/etc/passwd", "is absolute"},
		"an unclean path":            {"tests/Unit/", "is not clean"},
		"a path leaving the sandbox": {"../elsewhere", "leaves the sandbox"},
	} {
		t.Run(name, func(t *testing.T) {
			for _, args := range [][]string{
				{"test", fixturePR, "--repo", fixtureSlug},
				{"probe", "run", fixturePR, "--repo", fixtureSlug, "--kind", "mutation", "--patch", "unread.diff"},
			} {
				prepared, _, _, _ := probeFixture(t, passingRunner, pathsArgProfile)

				err := runCLI(t, append(args, "--path", tc.path)...)

				require.Error(t, err, "%v", args)
				assert.Equal(t, ExitUsage, exitCodeFor(err), "§5.2.1: a refused --path is §11.2's 2")
				assert.Contains(t, err.Error(), tc.refused)
				assert.Contains(t, err.Error(), tc.path)
				assert.NoDirExists(t, prepared.Sandbox(fixtureOwner, fixtureProject, fixturePRNumber),
					"the refusal lands before any state is read, so nothing ran")
			}
		})
	}

	t.Run("a path nothing exists at is admitted", func(t *testing.T) {
		probeFixture(t, passingRunner, pathsArgProfile)

		stdout, _ := streams(t, "test", fixturePR, "--repo", fixtureSlug, "--path", "tests/NotWrittenYet")

		assert.Equal(t, []any{"tests/NotWrittenYet"}, document(t, stdout)["paths"],
			"§5.2.1: cr does not check that a path exists")
	})
}

// §5.2.2 through the commands: a probe's baseline is "the run with the probe's
// filter and paths", so a `cr test` over the same paths is reused and one over
// other paths is not.
//
// The reuse is what makes the keying observable. Both stored runs passed and
// both stand at the head; the only thing that can tell them apart is the
// population they measured, and a resolver that ignored `paths` would hand the
// second probe a baseline measured over a directory its own run never touched.
func TestAPathedProbeReusesOnlyABaselineOverTheSamePaths(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, passingRunner, pathsArgProfile)
	patch := writePatch(t, fixtureDiff)

	streams(t, "test", fixturePR, "--repo", fixtureSlug, "--path", "tests/Unit")

	same := probeDocument(t, throughAPipe(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", patch, "--path", "tests/Unit"))
	assert.Equal(t, "r1", same["baseline"], "§5.2.2: the run over the same paths is the baseline")
	assert.Equal(t, "r2", same["run"])

	other := probeDocument(t, throughAPipe(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "mutation", "--patch", patch, "--path", "tests/Feature"))
	assert.Equal(t, "r3", other["baseline"], "§5.2.2: other paths are another population, so it is performed")
	assert.Equal(t, "r4", other["run"])

	runs := storedRecords(t, prepared, state.FileRuns)
	require.Len(t, runs, 4)
	assert.Equal(t, []any{"tests/Feature"}, runs[2]["paths"],
		"the baseline the second probe performed carries that probe's own paths")
	assert.NotContains(t, runs[2], "probe", "§5.2.6: only a run carrying no probe may serve as a baseline")
}

// §5.4.2: "The probe's own run MUST pass that directory through
// `tests.paths_arg` as its only path, its run record's `paths` holding it" —
// while §5.2.2 has its baseline measure "the run with the probe's paths and no
// filter".
//
// The two populations are different on purpose and both are asserted, because
// the point of the clause is that they differ: the placed file does not exist
// in the baseline, so a baseline narrowed to where it will go would still be a
// different set from the probe's, and a probe run narrowed to the whole
// `--path` set would run tests the ladder is not reading.
//
// v0.4.1 changed the probe's path from the placed file to its directory. The
// fixture's template puts the file in `tests`, so the assertion moves from
// `tests/cr_probe_p1.txt` to `tests`, and here it happens to coincide with the
// `--path` the baseline took — which is why the run records are read for it
// rather than the difference being inferred from the two values.
func TestAGapProbeRunsOverThePlacedDirectoryAndBaselinesOverItsPaths(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, gapProbeRunner, gapProbeTemplate, pathsArgProfile)
	supplied := writeProbeTest(t)

	reported := probeDocument(t, throughAPipe(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "gap", "--test", supplied, "--target", "app.go:3", "--path", "tests"))

	assert.Equal(t, []any{"tests"}, reported["paths"], "§5.5: the record carries the --path values")
	assert.Equal(t, "r1", reported["baseline"])
	assert.Equal(t, "r2", reported["run"])

	runs := storedRecords(t, prepared, state.FileRuns)
	require.Len(t, runs, 2)
	assert.Equal(t, []any{"tests"}, runs[0]["paths"],
		"§5.2.2: the baseline measures the probe's paths")
	assert.NotContains(t, runs[0], "filter", "and no filter, since the probe's test is not there yet")
	assert.Equal(t, []any{path.Dir(gapProbePath)}, runs[1]["paths"],
		"§5.4.2: the probe's own run is narrowed to the directory cr placed the file in")
	assert.NotContains(t, runs[1]["paths"], gapProbePath,
		"never the file itself: a runner that builds a package from its paths cannot build "+
			"one from a lone test file")
}

// §5.4.2: "When `--path` is given, at least one MUST lie under the directory of
// the path `tests.probe_path_template` gives, or the command aborts with exit
// code 2."
//
// The directory itself counts as under it, which is the ordinary invocation —
// a reviewer narrows to the directory the probe file goes in. What is refused
// is a `--path` set that reaches none of it, because the baseline would then
// measure one population and the probe run another.
func TestAGapProbeRefusesPathsThatReachNoneOfItsPlacement(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, gapProbeRunner, gapProbeTemplate, pathsArgProfile)
	supplied := writeProbeTest(t)

	err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "gap", "--test", supplied, "--target", "app.go:3", "--path", "src")

	require.Error(t, err)
	assert.Equal(t, ExitUsage, exitCodeFor(err), "§5.4.2: §11.2's 2")
	assert.Contains(t, err.Error(), gapProbePath, "the refusal names where the probe file would go")
	assert.Empty(t, storedRecords(t, prepared, state.FileRuns),
		"the refusal lands before §5.2.6 performs a baseline")
}

// §5.4.2's last clause: a profile with no `tests.paths_arg` has no way to name
// a path to its runner, so the probe's own run "MUST use the filter alone" and
// the runner's own discovery finds the placed file.
//
// This is the shape every profile cr ships has, laravel-pest included, so it is
// the path most gap probes take: the clause is not an edge case but the
// default.
func TestAGapProbeWithNoPathsArgUsesTheFilterAlone(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, gapProbeRunner, gapProbeTemplate)
	supplied := writeProbeTest(t)

	reported := probeDocument(t, throughAPipe(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--kind", "gap", "--test", supplied, "--target", "app.go:3", "--filter", "edge"))

	assert.NotContains(t, reported, "paths", "no --path was given, so the record carries none")

	runs := storedRecords(t, prepared, state.FileRuns)
	require.Len(t, runs, 2)
	assert.NotContains(t, runs[1], "paths",
		"§5.4.2: with no tests.paths_arg the probe's run names no path")
	assert.Equal(t, "edge", runs[1]["filter"], "§5.4.2: the filter alone")
	assert.Contains(t, runs[1]["output_tail"], gapProbeTest,
		"the runner's own discovery still found the placed file")
}
