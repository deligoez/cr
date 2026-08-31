package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sandboxFixture builds a repository whose head sits on a branch other than the
// checked-out one, and returns the repository, that head, and a path no
// worktree occupies yet.
//
// The two branches are what make the assertions below mean anything: a worktree
// added at the checked-out commit would look right whether or not the revision
// was honoured, and a repository with one branch cannot say whether the main
// worktree stayed where it was.
func sandboxFixture(t *testing.T) (repo, head, path string) {
	t.Helper()
	repo = fixtureRepo(t)

	writeFixtureFile(t, repo, "app.txt", "the base\n")
	fixtureGit(t, repo, "add", "app.txt")
	fixtureGit(t, repo, "commit", "--quiet", "-m", "the base")

	fixtureGit(t, repo, "checkout", "--quiet", "-b", "pr-head")
	writeFixtureFile(t, repo, "app.txt", "the change under review\n")
	fixtureGit(t, repo, "commit", "--quiet", "-a", "-m", "the change under review")
	head = fixtureGit(t, repo, "rev-parse", "HEAD")
	fixtureGit(t, repo, "checkout", "--quiet", "main")

	return repo, head, filepath.Join(t.TempDir(), "state", "sandbox")
}

// §5.1.1: the worktree is a checkout of the pull request head, and it is at the
// path cr named.
//
// The revision is asserted three ways, because they fail apart. HEAD says which
// commit was checked out; the file says the checkout really happened rather
// than only the pointer; and the absent symbolic HEAD says the worktree took no
// branch, which is §5.1.4's half of it — a branch checked out here is a branch
// the main worktree can no longer take.
func TestAddWorktreeChecksTheHeadOutAtTheGivenPath(t *testing.T) {
	repo, head, path := sandboxFixture(t)

	require.NoError(t, AddWorktree(repo, path, head))

	assert.Equal(t, head, fixtureGit(t, path, "rev-parse", "HEAD"))
	body, err := os.ReadFile(filepath.Join(path, "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, "the change under review\n", string(body))

	_, err = run(path, "symbolic-ref", "--quiet", "HEAD")
	assert.Error(t, err, "§5.1.4: the sandbox must take no branch of the repository under review")
}

// §5.1.4 and §2.2: adding the worktree moves nothing in the main checkout, and
// the registration is the only trace it leaves.
//
// The repository is dirtied first, and deliberately: a clean checkout would
// pass a comparison of `git status` for reasons of its own. What is compared is
// everything §2.2's second sentence names — the working tree against HEAD, the
// index, the commit HEAD sits at, the ref it follows, and every branch — read
// before and after the one call.
func TestAddWorktreeLeavesTheMainWorktreeAlone(t *testing.T) {
	repo, head, path := sandboxFixture(t)
	writeFixtureFile(t, repo, "app.txt", "edited by the author, uncommitted\n")
	writeFixtureFile(t, repo, "staged.txt", "staged and not committed\n")
	fixtureGit(t, repo, "add", "staged.txt")

	main := func() []string {
		t.Helper()
		return []string{
			fixtureGit(t, repo, "status", "--porcelain=v2", "--untracked-files=all"),
			fixtureGit(t, repo, "rev-parse", "HEAD"),
			fixtureGit(t, repo, "rev-parse", "--symbolic-full-name", "HEAD"),
			fixtureGit(t, repo, "for-each-ref", "--format=%(refname) %(objectname)"),
		}
	}
	before := main()

	require.NoError(t, AddWorktree(repo, path, head))

	assert.Equal(t, before, main(),
		"§5.1.4: cr must not modify the user's main worktree, index, or current branch")

	// §2.2's one exception, read directly: the registration git names
	// after the worktree's own directory.
	registration := filepath.Join(repo, ".git", "worktrees", filepath.Base(path))
	info, err := os.Stat(registration)
	require.NoError(t, err, "§5.1.1 registers the worktree, and nothing is registered")
	assert.True(t, info.IsDir())
}

// §5.1.5: removing the worktree takes its registration with it, and a dirty
// worktree is removed all the same.
//
// The dirt is the case §5.1.6 removes one in. A sandbox is rebuilt precisely
// because it failed a cleanliness check — an unreverted mutation, a leftover
// probe artefact — so a removal that refused a dirty worktree would refuse on
// every occasion cr has to remove one, and §5.1.6's mandated recreation could
// never happen at all.
//
// The registration is asserted separately from the directory because they fail
// apart: one left behind is what makes `worktree add` refuse the path next time,
// so a removal that took only the files would break the recreation it exists to
// serve, and only on the second run.
func TestRemoveWorktreeTakesTheRegistrationWithIt(t *testing.T) {
	repo, head, path := sandboxFixture(t)
	require.NoError(t, AddWorktree(repo, path, head))
	registration := filepath.Join(repo, ".git", "worktrees", filepath.Base(path))
	require.DirExists(t, registration)

	// An unreverted mutation, which is what a sandbox being removed holds.
	require.NoError(t, os.WriteFile(
		filepath.Join(path, "app.txt"), []byte("a mutation nobody reverted\n"), 0o600))

	require.NoError(t, RemoveWorktree(repo, path))

	assert.NoDirExists(t, path, "§5.1.5 removes the worktree")
	assert.NoDirExists(t, registration, "§5.1.5 removes its registration")
	assert.NoError(t, AddWorktree(repo, path, head),
		"the path stayed registered, so it can never be recreated")
}

// A registration whose directory is already gone is pruned, which is the case
// `worktree remove` cannot answer: it refuses a path that is not there, and the
// registration it left behind is what would refuse the next `worktree add`.
func TestPruneWorktreesClearsARegistrationWhoseDirectoryIsGone(t *testing.T) {
	repo, head, path := sandboxFixture(t)
	require.NoError(t, AddWorktree(repo, path, head))
	require.NoError(t, os.RemoveAll(path))

	require.NoError(t, PruneWorktrees(repo))

	assert.NoDirExists(t, filepath.Join(repo, ".git", "worktrees", filepath.Base(path)))
	assert.NoError(t, AddWorktree(repo, path, head))
}

// A removal that failed is reported, and does not become a prune that
// succeeded.
//
// The two steps are sequential rather than joined for exactly this reason.
// Pruning after a failed removal would hand back the prune's own result, so a
// worktree that is still there — held open, or at a path that is not a worktree
// at all — would be reported as removed, and §5.1.6's recreation would then
// fail at `worktree add` with a message about a path already registered, one
// step away from the thing that actually went wrong.
func TestARemovalThatFailedIsNotReportedAsSuccess(t *testing.T) {
	repo, _, path := sandboxFixture(t)

	err := RemoveWorktree(repo, path)

	var failed *CommandError
	require.ErrorAs(t, err, &failed, "nothing was registered at that path, so the removal cannot have worked")
	assert.Contains(t, failed.Args, "remove")
}
