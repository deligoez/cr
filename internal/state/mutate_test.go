package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The pull request whose sandbox these tests mutate.
const (
	mutateOwner = "octocat"
	mutateRepo  = "hello"
	mutatePR    = 7
)

// The production code these tests break, and what breaking it looks like.
const (
	mutableFile   = "app.go"
	mutableSource = "func Retry() { backoff() }\n"
	mutableBroken = "FUNC RETRY() { BACKOFF() }\n"
)

// mutableSandbox stands a sandbox up with one file in it and returns the layout
// and the file's path.
func mutableSandbox(t *testing.T) (layout Layout, path string) {
	t.Helper()
	layout = New(t.TempDir())
	sandbox := layout.Sandbox(mutateOwner, mutateRepo, mutatePR)
	require.NoError(t, os.MkdirAll(sandbox, 0o700))
	path = filepath.Join(sandbox, mutableFile)
	require.NoError(t, os.WriteFile(path, []byte(mutableSource), 0o600))
	return layout, path
}

// broken is the mutation these tests apply: production code the suite is meant
// to notice being broken.
func broken(content string) (string, error) {
	return strings.ToUpper(content), nil
}

// §5.3.3 and invariant 6: the mutation is reverted even when the run fails.
//
// The failing run is the ordinary case and the one a call-site revert would
// still get right, so what it proves is only that the wrapper works. The two
// cases beneath it are the ones the wrapper exists for.
func TestASandboxMutationIsRevertedAfterAFailingRun(t *testing.T) {
	layout, path := mutableSandbox(t)
	refused := errors.New("the suite refused to start")

	err := layout.UnderSandboxMutation(mutateOwner, mutateRepo, mutatePR,
		[]SandboxMutation{{Path: mutableFile, Apply: broken}}, func() error {
			held, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, mutableBroken, string(held),
				"§5.3.2: the mutation is on disk while the tests run")
			return refused
		})

	require.ErrorIs(t, err, refused, "the run's own failure reaches the caller")
	restored, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, mutableSource, string(restored),
		"§5.3.3: the mutation is reverted even when the run fails")
}

// A probe that ran without trouble reports no trouble.
//
// The restore's own failure is joined into what the caller is told, so the
// success path is the one that says the join is empty when nothing failed. A
// revert that reported a failure it did not have would turn every clean probe
// into a command that exited non-zero, and gremlins found that the negated
// condition inside the undo survived every other test in this package.
func TestASandboxMutationThatRanCleanlyReportsNothing(t *testing.T) {
	layout, path := mutableSandbox(t)

	require.NoError(t, layout.UnderSandboxMutation(mutateOwner, mutateRepo, mutatePR,
		[]SandboxMutation{{Path: mutableFile, Apply: broken}}, func() error { return nil }))

	restored, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, mutableSource, string(restored))
}

// §5.3.3 and invariant 6: the mutation is reverted when the run panics, and the
// panic still propagates.
//
// This is the case a revert written after the run cannot answer at all, and it
// is not hypothetical: a probe run walks an agent-supplied patch, a runner's
// output, and a profile's regular expressions, and every one of those is a
// place a nil dereference has come from before. Both halves are asserted —
// a wrapper that swallowed the panic to do its cleanup would leave the caller
// believing a probe succeeded that never finished.
func TestASandboxMutationIsRevertedWhenTheRunPanics(t *testing.T) {
	layout, path := mutableSandbox(t)

	assert.PanicsWithValue(t, "the runner came apart", func() {
		_ = layout.UnderSandboxMutation(mutateOwner, mutateRepo, mutatePR,
			[]SandboxMutation{{Path: mutableFile, Apply: broken}}, func() error {
				panic("the runner came apart")
			})
	}, "the panic still reaches the caller")

	restored, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, mutableSource, string(restored),
		"§5.3.3: the mutation is reverted even when the run comes apart")
}

// A patch that applies to one file and not the next leaves neither mutated.
//
// The caller receives no undo when the apply itself fails — there was nothing
// to hand back — so the obligation is discharged inside, and the file that was
// already rewritten is put back before the error returns. Without it §5.3.4's
// first rung would be recorded against a sandbox still holding half a mutation,
// and §5.1.6 would find it on the next run.
func TestAMutationThatFailsPartwayLeavesNothingBehind(t *testing.T) {
	layout, path := mutableSandbox(t)
	sandbox := layout.Sandbox(mutateOwner, mutateRepo, mutatePR)
	second := filepath.Join(sandbox, "other.go")
	require.NoError(t, os.WriteFile(second, []byte("func Other() {}\n"), 0o600))
	refused := errors.New("the hunk does not match the file")

	err := layout.UnderSandboxMutation(mutateOwner, mutateRepo, mutatePR, []SandboxMutation{
		{Path: mutableFile, Apply: broken},
		{Path: "other.go", Apply: func(string) (string, error) { return "", refused }},
	}, func() error {
		t.Error("§5.3.4's first rung runs no tests, so the run must not have been reached")
		return nil
	})

	require.ErrorIs(t, err, refused)
	restored, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, mutableSource, string(restored),
		"the file the apply had already rewritten is put back")
	untouched, readErr := os.ReadFile(second)
	require.NoError(t, readErr)
	assert.Equal(t, "func Other() {}\n", string(untouched))
}

// §5.1.4 and invariant 2: a path that does not land inside the sandbox is
// refused, and nothing is written.
//
// The sandbox is a worktree of the repository under review, so `..` in an
// agent-written diff arrives in the reviewer's own checkout. The symbolic link
// is the case a lexical check misses: `sandbox.copy` brings in trees full of
// them, and a patch addressing a path through one would be written wherever it
// points.
func TestAPathThatLeavesTheSandboxIsRefused(t *testing.T) {
	layout, path := mutableSandbox(t)
	sandbox := layout.Sandbox(mutateOwner, mutateRepo, mutatePR)

	outside := filepath.Join(t.TempDir(), "elsewhere.go")
	require.NoError(t, os.WriteFile(outside, []byte("func Elsewhere() {}\n"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(sandbox, "linked.go")))

	for name, rel := range map[string]string{
		"a path that climbs out":   "../../escaped.go",
		"an absolute path":         outside,
		"a path through a symlink": "linked.go",
		"a path naming no file":    "",
	} {
		t.Run(name, func(t *testing.T) {
			err := layout.UnderSandboxMutation(mutateOwner, mutateRepo, mutatePR,
				[]SandboxMutation{{Path: rel, Apply: broken}}, func() error {
					t.Error("a refused path must not reach the run")
					return nil
				})

			var refused *OutsideSandboxError
			require.ErrorAs(t, err, &refused)
			assert.Contains(t, refused.Error(), "§5.1.4")
		})
	}

	held, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "func Elsewhere() {}\n", string(held), "nothing outside the sandbox was written")
	untouched, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, mutableSource, string(untouched))
}
