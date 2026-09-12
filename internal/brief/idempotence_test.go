package brief

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
)

// snapshot is every regular file under the state root, keyed by its path
// relative to that root and holding its bytes.
//
// The whole root is walked rather than the §2.3 rows of one pull request,
// because §9.3.3's "idempotent" is a claim about what the command leaves
// behind and not about the files somebody remembered to list. A brief that
// started appending to context/<ISSUE-KEY>.ndjson, to a waivers file, or to a
// rounds/<n>/ artefact would satisfy a row-by-row comparison and still have
// changed the state directory.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, readErr := os.ReadFile(path) //nolint:gosec // a path the walk produced
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		out[rel] = string(body)
		return nil
	}))
	return out
}

// A second `cr brief` at an unchanged head leaves the state directory
// byte-identical, and a `cr brief` at a moved head opens the next round.
//
// The two halves are one test because §9.3.3 is one condition read from both
// sides: the index moves if and only if the head differs. Asserting only the
// first half would pass on a brief that never moves the round at all, which is
// idempotent for the trivial reason that it is inert — and that was this
// package's behaviour before, `roundOf` having returned the recorded index
// whatever the head said.
//
// Byte equality is the assertion rather than a field-by-field comparison
// because §9.3.3 is about the whole directory: units.ndjson's ordering, the
// serialised form of meta.json, and the thread page are each something a
// re-run could perturb without changing any value a reader would think to
// check.
func TestASecondBriefAtOneHeadChangesNothingAndAMovedHeadOpensTheNextRound(t *testing.T) {
	dir, head, base := repository(t)
	src := sources(t, dir, answering(head, base, oneThread))

	first, err := Run(src)
	require.NoError(t, err)
	require.Equal(t, 1, first.Round)

	before := snapshot(t, src.Layout.Root())
	second, err := Run(src)
	require.NoError(t, err)

	assert.Equal(t, 1, second.Round, "§9.3.3: the head has not moved, so the round is the one already open")
	assert.Equal(t, before, snapshot(t, src.Layout.Root()),
		"§9.3.3: a same-head brief is idempotent, so it leaves the state directory as it found it")

	// The one case §3.7 allows a brief to change state: a new head.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "order.go"),
		[]byte("package shop\n\nfunc Total() int { return subtotal() + shipping() + tax() }\n"), 0o600))
	runGit(t, dir, "add", "order.go")
	runGit(t, dir, "commit", "--quiet", "-m", "tax the order")
	moved := runGit(t, dir, "rev-parse", "HEAD")
	require.NotEqual(t, head, moved)

	src.GH = gh.WithRunner(answering(moved, base, oneThread))
	third, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, 2, third.Round, "§9.3.3: the head moved, so the round index moves with it")

	recorded, err := src.Layout.Briefed(testOwner, testRepo, testPR)
	require.NoError(t, err)
	assert.Equal(t, 2, recorded.Round)
	assert.Equal(t, moved, recorded.Head, "§9.3.3 stores the new head alongside the new index")
}
