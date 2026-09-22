package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// §5.4.2 passes the directory the placed file sits in, and the repository root
// is a directory like any other.
//
// v0.6.0 handed a root-placed file to the runner itself. Measured 2026-09-22 on
// a Go module whose package is at the root: `go test ./cr_probe_p2_test.go`
// compiled the file alone, failed on the function it tested, and every gap
// probe there came back `inconclusive`; with `.` the same probes answered
// `passed` and `failed`.
func TestAGapProbeAtTheRootRunsTheRootDirectory(t *testing.T) {
	for placement, want := range map[string]string{
		"cr_probe_p2_test.go":                ".",
		"internal/probe/cr_probe_p1_test.go": "internal/probe",
		"tests/Feature/cr_probe_p3Test.php":  "tests/Feature",
	} {
		assert.Equal(t, want, probeRunPath(placement), placement)
	}
}
