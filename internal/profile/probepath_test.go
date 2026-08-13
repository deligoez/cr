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

// The template's placeholder vocabulary is closed to <probe-id> and <ext>. A
// token cr does not substitute would reach the sandbox literally, so the file
// §5.4.2 places and the file §5.1.6 looks for would be two different names, and
// a leftover artefact would go unnoticed for the rest of the review.
func TestProbePathTemplateVocabularyIsClosed(t *testing.T) {
	block := `{"cmd": ["make", "test"], "globs": ["tests/*Test.php"], "probe_path_template": %q}`

	// <ext> is the second member of the vocabulary, and it resolves from
	// tests.globs with no test file supplied.
	p, err := Load(testsProfile(t, fmt.Sprintf(block, "tests/Feature/cr_probe_<probe-id><ext>")))
	require.NoError(t, err)
	assert.Equal(t, "tests/Feature/cr_probe_<probe-id>Test.php", p.Tests.ProbePathTemplate)

	for name, template := range map[string]string{
		"an invented token":  "tests/cr_probe_<probe-id>_<slug>.php",
		"a misspelt one":     "tests/cr_probe_<probeid>.php",
		"a plausible cousin": "tests/cr_probe_<probe-id><extension>",
	} {
		t.Run(name, func(t *testing.T) {
			path := testsProfile(t, fmt.Sprintf(block, template))

			_, err := Load(path)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, "tests.probe_path_template", malformed.Field)
			assert.Contains(t, err.Error(), path)
		})
	}
}

// §5.1.6 recognises a leftover artefact by replacing the template's <probe-id>
// with *, so the template must contain exactly one. None leaves every probe
// writing the same path, and two leave a glob that no longer names one file.
func TestProbePathTemplateNeedsExactlyOneProbeID(t *testing.T) {
	block := `{"cmd": ["make", "test"], "globs": ["tests/*Test.php"], "probe_path_template": %q}`

	for name, template := range map[string]string{
		"none": "tests/cr_probe.php",
		"two":  "tests/<probe-id>/cr_probe_<probe-id>.php",
	} {
		t.Run(name, func(t *testing.T) {
			path := testsProfile(t, fmt.Sprintf(block, template))

			_, err := Load(path)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, "tests.probe_path_template", malformed.Field)
			assert.Contains(t, err.Error(), "<probe-id>")
			assert.Contains(t, err.Error(), path)
		})
	}
}

