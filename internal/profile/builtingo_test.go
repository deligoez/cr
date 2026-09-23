package profile

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
)

// loadGo parses the shipped Go profile with the loader a user's own file goes
// through.
func loadGo(t *testing.T) Profile {
	t.Helper()
	p, err := Parse(goID+fileExt, []byte(goProfile))
	require.NoError(t, err)
	return p
}

// §2.4.5: the Go profile carries what the probe machinery needs to work at all
// — a runner, readable counts, a filter flag and a placeable probe path — and
// each field is asserted at its value, for the reason laravel-pest's are.
func TestTheShippedGoProfileFillsEveryFieldItNeeds(t *testing.T) {
	p := loadGo(t)

	assert.Equal(t, goID, p.ID)
	assert.Equal(t, []string{"go.mod", "go.sum"}, p.Match.Files,
		"§2.4.5: two markers, so a Go module carrying a package.json is not a §2.4.2 tie with typescript")
	for _, id := range axis.IDs() {
		assert.True(t, p.Axes[id], "axes must enable %s", id)
	}
	assert.Equal(t, []string{"go", "test", "-v", "-count=1"}, p.Tests.Cmd)
	assert.Equal(t, "-run", p.Tests.FilterFlag)
	assert.True(t, p.CountsOccurrences(), "go test prints no recap line to sum")
	assert.Equal(t, "go", p.Symbols.Lang)
	assert.Equal(t, []string{}, p.Sandbox.Require)
}

// §5.2.1's argv: a bare run tests every package, and a path names a package.
//
// `go test` with no package argument tests only the current directory, so
// without `tests.paths_default` the baseline §5.2.2 compares every probe
// against would be a run of the module root's package alone.
func TestTheGoProfileRunsTheWholeModuleUnlessAPathIsGiven(t *testing.T) {
	p := loadGo(t)

	whole, err := p.TestArgv(goID+fileExt, "", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"go", "test", "-v", "-count=1", "./..."}, whole)

	scoped, err := p.TestArgv(goID+fileExt, "TestLoad", []string{"internal/probe"})
	require.NoError(t, err)
	assert.Equal(t, []string{"go", "test", "-v", "-count=1", "-run", "TestLoad", "./internal/probe"}, scoped)
}

// §5.4.2: a Go test compiles into the package it tests, so the probe file is
// placed beside its target and still satisfies the profile's own test glob.
func TestTheGoProbePathLandsInTheTargetsPackage(t *testing.T) {
	p := loadGo(t)

	placed := p.ProbePath("p3", "internal/probe")

	assert.Equal(t, "internal/probe/cr_probe_p3_test.go", placed)
	assert.True(t, strings.HasSuffix(placed, "_test.go"), "go test collects only _test.go files")
}

// The files under testdata/go are the real output of go1.27.1 on darwin/arm64,
// captured 2026-09-22 from a two-package module rather than written, with the
// exit status each run returned.
//
// Four readings matter. A failing run counts its top-level tests once each,
// including a parent whose subtest failed, and not the indented subtest lines.
// A skipped test is not an executed one, so a run of skips alone reads zero —
// the answer for a run that selected nothing — rather than a run in which
// nothing failed. A filter that matched nothing exits 0 with no test line, and
// is zero. And a tree that did not build also prints no test line but exits 1,
// and is undetermined: reading it as zero would put §5.3.4's ladder on
// `no-tests-selected`, the answer for a filter, about code that did not
// compile.
func TestTheGoPatternsCountCapturedGoTestOutput(t *testing.T) {
	p := loadGo(t)

	cases := map[string]struct {
		file     string
		exit     int
		executed string
		failed   string
	}{
		"an all-passing run": {"all-passing.txt", 0, "1", "0"},
		// TestAddPasses, TestAddSubtests (whose subtest failed), TestMul;
		// TestAddSkipped is skipped.
		"a failing run":          {"some-failing.txt", 1, "3", "1"},
		"a filter matching none": {"no-tests-selected.txt", 0, "0", "0"},
		"only skipped tests":     {"skipped-only.txt", 0, "0", "0"},
		"a tree that did not build": {
			"build-failed.txt", 1, "undetermined", "undetermined"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			output, err := os.ReadFile("testdata/go/" + tc.file)
			require.NoError(t, err)

			executed, failed := counted(t, &p, string(output), tc.exit)
			assert.Equal(t, tc.executed, executed, "executed count")
			assert.Equal(t, tc.failed, failed, "failed count")
		})
	}

	// The build-failure case rests on there being no test line at all, so
	// the premise is asserted rather than assumed.
	broken, err := os.ReadFile("testdata/go/build-failed.txt")
	require.NoError(t, err)
	assert.NotContains(t, string(broken), "--- ")
	assert.Contains(t, string(broken), "[build failed]")
}
