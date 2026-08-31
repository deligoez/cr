package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Where §2.4's template puts a gap probe's test in these tests, and what §5.4.1
// has the agent supply.
//
// The path names two directories the sandbox does not hold, because that is the
// ordinary shape: §2.4 roots the default at the first `tests.globs` entry, so
// the placement usually has to make a directory and the removal has to take it
// away again.
const (
	probeRel  = "tests/nested/cr_probe_p1.txt"
	probeBody = "the supplied test, asserting the edge case\n"
)

// §5.4.2: the test is in the sandbox while the run happens, and off disk
// afterwards — together with the directories the placement had to create.
//
// The directories matter as much as the file. §5.1.6's leftover scan looks for
// the file, but a sandbox left carrying an empty tree cr made is no longer the
// sandbox the post-setup baseline described, and the next run would be
// measuring a checkout nobody put back.
func TestAProbeTestFileIsPlacedWhereTheTemplateSaysAndRemovedAfter(t *testing.T) {
	layout, _ := mutableSandbox(t)
	box := layout.Sandbox(mutateOwner, mutateRepo, mutatePR)
	placed := filepath.Join(box, filepath.FromSlash(probeRel))

	require.NoError(t, layout.UnderSandboxTestFile(mutateOwner, mutateRepo, mutatePR,
		probeRel, probeBody, func() error {
			held, err := os.ReadFile(placed)
			require.NoError(t, err, "§5.4.2: the test is in the sandbox while it runs")
			assert.Equal(t, probeBody, string(held))
			return nil
		}))

	assert.NoFileExists(t, placed, "§5.4.2: the test is removed after the run")
	assert.NoDirExists(t, filepath.Join(box, "tests"),
		"the directories the placement made go with the file it made them for")
}

// §5.4.2's abort, and §5.1.4's boundary around the path it aborts on.
//
// The first row is the one §5.4.2 names: a file already at the probe path is
// refused rather than written over, because the placement removes what it wrote
// and would therefore destroy a real test and then delete it.
//
// The rest are the same refusal the mutation side makes about a patch's paths,
// asked of a template instead. `tests.probe_path_template` is a field the user
// writes, so `../` is a value cr will be handed sooner or later — and the
// sandbox is a worktree of the repository under review, so one `..` arrives in
// the checkout the reviewer is working in, which invariant 2 permits cr no
// write into at all. The last row is the half a lexical check misses: a
// directory inside the sandbox that is a link to somewhere outside it.
//
// Every row asserts that the run never happened. A refusal that came after the
// suite had been started would have spent the whole of §5.2.6's baseline on an
// experiment cr was never going to perform.
func TestAProbePathThatIsTakenOrLeavesTheSandboxIsRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		rel  string
		says string
		// arrange prepares the sandbox for rows that need more than a
		// path, and is nil for the rows that do not.
		arrange func(t *testing.T, box string) string
	}{
		"a file is already standing there": {
			rel:  mutableFile,
			says: mutableFile + " already exists",
		},
		"it leaves the tree it is placed in": {
			rel:  "../escaped_p1.txt",
			says: "it leaves the tree it is placed in",
		},
		"it is absolute": {
			rel:  string(filepath.Separator) + filepath.Join("tmp", "escaped_p1.txt"),
			says: "it is absolute",
		},
		"it names no file": {
			rel:  "",
			says: "it names no file",
		},
		"its nearest directory is a link out of the tree": {
			rel:  "linked/cr_probe_p1.txt",
			says: "its nearest existing directory resolves to",
			arrange: func(t *testing.T, box string) string {
				t.Helper()
				outside := t.TempDir()
				require.NoError(t, os.Symlink(outside, filepath.Join(box, "linked")))
				return filepath.Join(outside, "cr_probe_p1.txt")
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			layout, _ := mutableSandbox(t)
			box := layout.Sandbox(mutateOwner, mutateRepo, mutatePR)
			unwritten := ""
			if tc.arrange != nil {
				unwritten = tc.arrange(t, box)
			}

			ran := false
			err := layout.UnderSandboxTestFile(mutateOwner, mutateRepo, mutatePR,
				tc.rel, probeBody, func() error {
					ran = true
					return nil
				})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.says)
			assert.False(t, ran, "the refusal comes before anything is run")
			if unwritten != "" {
				assert.NoFileExists(t, unwritten, "nothing was written outside the sandbox")
			}
		})
	}

	// The file the first row refused over is still the file it was: the
	// abort leaves it alone rather than removing what it declined to
	// write.
	layout, path := mutableSandbox(t)
	require.Error(t, layout.UnderSandboxTestFile(mutateOwner, mutateRepo, mutatePR,
		mutableFile, probeBody, func() error { return nil }))
	held, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, mutableSource, string(held),
		"§5.4.2 aborts rather than removing the file it refused to write over")
}
