package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §5.4.2's `<target-dir>` template resolves §5.1.6's glob to one opening `**/`,
// because a scan runs with no probe and therefore no target directory in hand.
//
// `filepath.Glob` reads `**` as a single segment, so a glob evaluated that way
// finds a leftover one directory down and misses one two directories down —
// and a sandbox holding an artefact would be called clean, which is the miss
// §5.1.6 exists to prevent. Both depths are asserted here, because the shallow
// one passes under either implementation and only the deep one separates them.
func TestALeftoverIsFoundAtAnyDepthUnderAnAnyDepthGlob(t *testing.T) {
	const anyDepth = "**/cr_probe_*.txt"

	for name, within := range map[string]string{
		"at the root":            "cr_probe_p1.txt",
		"one directory down":     "pkg/cr_probe_p1.txt",
		"two directories down":   "pkg/inner/cr_probe_p1.txt",
		"three directories down": "pkg/inner/deeper/cr_probe_p1.txt",
	} {
		t.Run(name, func(t *testing.T) {
			src, path, _ := sandboxed(t)
			artefact := filepath.Join(path, filepath.FromSlash(within))
			require.NoError(t, os.MkdirAll(filepath.Dir(artefact), 0o700))
			require.NoError(t, os.WriteFile(artefact, []byte("a gap probe's test\n"), 0o600))

			ready, err := Ensure(src, anyDepth)
			require.NoError(t, err)

			require.NotNil(t, ready.Recreated, "§5.1.6: the leftover is what makes it unclean")
			assert.Contains(t, ready.Recreated.Reason, within)
		})
	}
}

// The walk stays quiet on a sandbox holding nothing that matches: a scan that
// named something here would recreate every sandbox on every run, which
// §5.1.6's convergence forbids.
//
// A sandbox is a git worktree, so its `.git` is a *file* pointing at the
// repository's `.git/worktrees/<name>` — measured here, where an assertion
// written as DirExists failed. The walk therefore never descends into git's
// administrative files at all, and the `.git` directory skip it carries is for
// the case where the path handed to it is an ordinary clone.
func TestAnAnyDepthGlobMatchingNothingLeavesTheSandboxClean(t *testing.T) {
	src, path, sentinel := sandboxed(t)
	require.FileExists(t, filepath.Join(path, ".git"),
		"a worktree's .git is a file, so there is no git directory under the sandbox to walk")

	ready, err := Ensure(src, "**/cr_probe_*.txt")
	require.NoError(t, err)

	assert.Nil(t, ready.Recreated)
	assert.FileExists(t, sentinel, "the untracked files sandbox.copy and sandbox.setup create stand")
}
