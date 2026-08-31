package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// The pull request every fixture here stands for.
const (
	fixtureOwner = "acme"
	fixtureRepo  = "web"
	fixturePR    = 42
)

// runGit runs one command in a fixture repository and returns its trimmed output.
// A failure fails the test: a fixture that did not build proves nothing about
// the code under test.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// repository builds the repository under review and returns it with the head
// the pull request is at.
//
// The head is on a branch other than the checked-out one, so a sandbox created
// at the wrong revision is visible rather than accidentally right.
func repository(t *testing.T) (dir, head string) {
	t.Helper()
	dir = t.TempDir()
	runGit(t, dir, "init", "--quiet", "--initial-branch=main")
	runGit(t, dir, "config", "user.email", "fixture@cr.test")
	runGit(t, dir, "config", "user.name", "cr fixture")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.txt"), []byte("the base\n"), 0o600))
	runGit(t, dir, "add", "app.txt")
	runGit(t, dir, "commit", "--quiet", "-m", "the base")

	runGit(t, dir, "checkout", "--quiet", "-b", "pr-head")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.txt"), []byte("under review\n"), 0o600))
	runGit(t, dir, "commit", "--quiet", "-a", "-m", "the change under review")
	head = runGit(t, dir, "rev-parse", "HEAD")
	runGit(t, dir, "checkout", "--quiet", "main")

	return dir, head
}

// sources is the fixture's Sources, with a state root of its own so no test
// touches a real ~/.cr.
func sources(t *testing.T, dir, head string) *Sources {
	t.Helper()
	return &Sources{
		Layout:  state.New(filepath.Join(t.TempDir(), ".cr")),
		Owner:   fixtureOwner,
		Repo:    fixtureRepo,
		PR:      fixturePR,
		Head:    head,
		RepoDir: dir,
	}
}

// §5.1.1: the sandbox is a worktree at the pull request head, under the pull
// request's own state directory.
//
// The reported path and the path on disk are both checked against the layout's
// answer, because a Result that named one directory while git wrote another
// would leave every later probe running somewhere the report never mentioned.
func TestCreateMakesTheWorktreeAtTheHead(t *testing.T) {
	dir, head := repository(t)
	src := sources(t, dir, head)

	created, err := Create(src)
	require.NoError(t, err)

	assert.Equal(t, src.Layout.Sandbox(fixtureOwner, fixtureRepo, fixturePR), created.Path)
	assert.Equal(t, head, created.Head)
	assert.Equal(t, head, runGit(t, created.Path, "rev-parse", "HEAD"))

	body, err := os.ReadFile(filepath.Join(created.Path, "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, "under review\n", string(body))
}
