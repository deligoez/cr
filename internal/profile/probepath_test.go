package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testsProfile writes a well-formed profile carrying the given tests block, so
// each case shows only the block it is about.
func testsProfile(t *testing.T, tests string) string {
	t.Helper()
	return write(t, "laravel-pest", fmt.Sprintf(`{
		"id": "laravel-pest",
		"match": {"files": ["artisan"], "globs": ["app/**/*.php"]},
		"axes": {"test": true},
		"tests": %s
	}`, tests))
}

// §2.4 defaults tests.probe_path_template to
// <root of the first tests.globs entry>/cr_probe_<probe-id><ext>. The root is
// the longest leading path prefix free of *, ? and [, and <ext> is the literal
// suffix following the last wildcard segment of that same entry — both read from
// the profile alone, because §5.1.6 resolves the template with no test file in
// hand.
func TestProbePathTemplateDefaultsToTheFirstTestsGlob(t *testing.T) {
	cases := map[string]struct {
		globs    string
		resolved string
	}{
		"a root and a suffix":                 {`["tests/**/*Test.php"]`, "tests/cr_probe_<probe-id>Test.php"},
		"the root ends on a segment boundary": {`["tests/Feature*/*.php"]`, "tests/cr_probe_<probe-id>.php"},
		"no root at all":                      {`["*_test.go"]`, "cr_probe_<probe-id>_test.go"},
		"nothing follows the last wildcard":   {`["spec/**"]`, "spec/cr_probe_<probe-id>"},
		// A glob with no wildcard has no suffix to take and is its own
		// root. The mechanism stays literal rather than guessing, and a
		// profile in that shape sets the template explicitly.
		"no wildcard at all":            {`["test/all_test.exs"]`, "test/all_test.exs/cr_probe_<probe-id>"},
		"only the first entry seeds it": {`["tests/Unit/*Test.php", "tests/Feature/*.php"]`, "tests/Unit/cr_probe_<probe-id>Test.php"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := testsProfile(t, fmt.Sprintf(`{"cmd": ["make", "test"], "globs": %s}`, tc.globs))

			p, err := Load(path)

			require.NoError(t, err)
			assert.Equal(t, tc.resolved, p.Tests.ProbePathTemplate)
		})
	}
}

