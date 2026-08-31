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
