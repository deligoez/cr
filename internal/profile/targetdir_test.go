package profile

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// block is probepath_test.go's fixture shape, kept here so these cases read on
// their own: a runner, the globs `<ext>` resolves from, and the template.
const block = `{"cmd": ["make", "test"], "globs": ["tests/*Test.php"], "probe_path_template": %q}`

// §5.4.2 in v0.4.1: a template's directory may be `<target-dir>`, resolved to
// the directory of the path in `--target`.
//
// It exists because a language that compiles a test into the package it tests
// has no one directory that serves every probe. Measurement 4 part B placed a
// `package probe` test at `internal/cli/cr_probe_p2_test.go` — the one path the
// Go profile's template could give — and `go test` exited 1 on
// `undefined: Target` without running a test.
func TestAProbePathMayFollowItsTargetsDirectory(t *testing.T) {
	p, err := Load(testsProfile(t, fmt.Sprintf(block, "<target-dir>/cr_probe_<probe-id>_test.go")))
	require.NoError(t, err)

	assert.Equal(t, "internal/probe/cr_probe_p4_test.go", p.ProbePath("p4", "internal/probe"))
	assert.Equal(t, "internal/draft/cr_probe_p4_test.go", p.ProbePath("p4", "internal/draft"),
		"the same profile places two probes in two packages, which is the whole point")
	assert.True(t, p.TakesTargetDir())

	// A target at the repository root leaves the file there rather than
	// under a leading separator, which would be a path outside the sandbox.
	assert.Equal(t, "cr_probe_p4_test.go", p.ProbePath("p4", ""))
	assert.Equal(t, "cr_probe_p4_test.go", p.ProbePath("p4", "."))
}

// A template fixed whole ignores the target directory entirely, which is every
// profile cr ships. The placeholder is opt-in and changes nothing without it.
func TestAFixedProbePathIgnoresTheTargetDirectory(t *testing.T) {
	p, err := Load(testsProfile(t, fmt.Sprintf(block, "tests/Feature/cr_probe_<probe-id>.php")))
	require.NoError(t, err)

	assert.Equal(t, "tests/Feature/cr_probe_p4.php", p.ProbePath("p4", "app/Services"))
	assert.False(t, p.TakesTargetDir())
}

// §5.4.2: "a template carrying that placeholder in any other position MUST
// abort with exit code 2."
//
// The placeholder stands for the whole leading directory, so anywhere else it
// would name half a path — and §5.1.6, which scans without a target in hand,
// could not turn it back into something to look for.
func TestAProbePathRefusesTargetDirOutsideItsFirstSegment(t *testing.T) {
	for name, template := range map[string]string{
		"a second segment":     "tests/<target-dir>/cr_probe_<probe-id>.php",
		"inside the file name": "tests/<target-dir>cr_probe_<probe-id>.php",
		"twice":                "<target-dir>/<target-dir>/cr_probe_<probe-id>.php",
		"the whole path":       "<target-dir>",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(testsProfile(t, fmt.Sprintf(block, template)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "<target-dir>")
			assert.Contains(t, err.Error(), "first segment")
		})
	}
}

// §5.1.6 scans a sandbox with no probe running, so it has no target directory
// to resolve. The glob says "any directory" with `**`, and internal/sandbox is
// what decides to walk rather than glob it — `filepath.Glob` reads `**` as one
// segment and would miss a leftover two directories down.
func TestTheLeftoverGlobReachesEveryTargetDirectory(t *testing.T) {
	p, err := Load(testsProfile(t, fmt.Sprintf(block, "<target-dir>/cr_probe_<probe-id>_test.go")))
	require.NoError(t, err)

	assert.Equal(t, "**/cr_probe_*_test.go", p.LeftoverGlob())
}
