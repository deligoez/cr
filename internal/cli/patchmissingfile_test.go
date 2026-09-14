package cli

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// §5.3.1's mutation is a diff against a sandbox file, and §11.2 codes a patch
// whose data is wrong 1.
//
// QA found a patch naming a file the sandbox does not hold refused with exit 2
// and the usage hint, from an lstat error no row of the exit table mapped: the
// flags were right and the patch file was read, so nothing about the
// invocation was wrong. It is now refused with 1, naming the patch file, the
// line of the file header and the path, before any suite runs and before
// anything is recorded — whether the missing file is the patch's only one, a
// later file section behind one that exists, or a path through a directory
// the sandbox does not hold.
func TestAPatchNamingAFileTheSandboxLacksIsRefusedWithOne(t *testing.T) {
	const step = "correct the file header on the line the message names so it names a file the " +
		"pull request's head holds, as a path from the repository root"
	section := func(path string) string {
		return "--- a/" + path + "\n+++ b/" + path + "\n@@ -1,1 +1,1 @@\n-package gone\n+package went\n"
	}
	cases := map[string]struct {
		patch string
		line  int
		path  string
	}{
		"the patch's only file":         {patch: section("gone.go"), line: 2, path: "gone.go"},
		"a file behind one that exists": {patch: fixtureDiff + section("gone.go"), line: 9, path: "gone.go"},
		"a directory the sandbox lacks": {patch: section("lib/gone.go"), line: 2, path: "lib/gone.go"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			prepared, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
			patch := writePatch(t, c.patch)

			err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
				"--kind", "mutation", "--patch", patch)
			var malformed *git.MalformedPatchError
			require.True(t, errors.As(err, &malformed), "refused as the patch's data: %v", err)
			assert.Equal(t, fmt.Sprintf(
				"%s line %d: the file header names %q, which is not a file the sandbox holds: a probe "+
					"applies hunks only to files already there, and a file it created would have no "+
					"original to put back", patch, c.line, c.path), err.Error())
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Equal(t, step, hintFor(err))
			assert.NoFileExists(t, log, "the refusal comes before any suite is run")
			assert.Empty(t, storedRecords(t, prepared, state.FileProbes), "nothing is recorded")
		})
	}

	// The sandbox's own refusal, should one reach the writer rather than
	// the check ahead of it, takes the same code and step.
	absent := &state.AbsentFromSandboxError{Path: "gone.go", Err: os.ErrNotExist}
	assert.Equal(t, ExitValidation, exitCodeFor(absent))
	assert.Equal(t, step, hintFor(absent))
}
