package run

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §5.2.1: "Every `--path` MUST be relative, clean, and resolve inside the
// sandbox, or the command aborts with exit code 2; `cr` MUST NOT check that it
// exists."
//
// The last clause is asserted as loudly as the first three, and it is the one
// an implementation drifts away from: a path check that stats the file would
// refuse a directory the profile's setup has yet to create, and would do it
// from a sandbox that §5.1.6 may be about to rebuild.
func TestAPathMustBeRelativeCleanAndInsideTheSandbox(t *testing.T) {
	for name, tc := range map[string]struct {
		path    string
		refused string
	}{
		"a directory inside the sandbox":     {"tests/Feature", ""},
		"a file inside the sandbox":          {"tests/Unit/CartTest.php", ""},
		"the sandbox root itself":            {".", ""},
		"a path nothing exists at":           {"tests/NotWrittenYet", ""},
		"a name that is not a file at all":   {"unit", ""},
		"an absolute path":                   {"/etc/passwd", "is absolute"},
		"an absolute path inside the tree":   {"/tests/Unit", "is absolute"},
		"an empty path":                      {"", "is not clean"},
		"a trailing separator":               {"tests/Unit/", "is not clean"},
		"a path with a redundant segment":    {"tests/./Unit", "is not clean"},
		"a path that climbs and comes back":  {"tests/../tests/Unit", "is not clean"},
		"a parent directory":                 {"..", "leaves the sandbox"},
		"a path under the parent directory":  {"../other/tests", "leaves the sandbox"},
		"a path climbing above the sandbox":  {"../../etc", "leaves the sandbox"},
		"a hidden directory inside":          {".github/workflows", ""},
		"a path whose name begins with dots": {"..hidden/tests", ""},
	} {
		t.Run(name, func(t *testing.T) {
			err := CheckPaths([]string{tc.path})
			if tc.refused == "" {
				require.NoError(t, err, "§5.2.1 admits this path, and never asks whether it exists")
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.refused)
			assert.Contains(t, err.Error(), tc.path,
				"§12.4: the refusal names the value the reader has to correct")
		})
	}
}

// Every path is checked and not only the first, and no path at all is no
// refusal: §5.2.1's conditions are on each `--path`, and a command given none
// is the unnarrowed run every earlier release performed.
func TestEveryPathIsCheckedAndNoPathIsNoRefusal(t *testing.T) {
	require.NoError(t, CheckPaths(nil))
	require.NoError(t, CheckPaths([]string{}))
	require.NoError(t, CheckPaths([]string{"tests/Unit", "tests/Feature"}))

	err := CheckPaths([]string{"tests/Unit", "../elsewhere"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "../elsewhere",
		"a later path is refused as the first one is")
}
