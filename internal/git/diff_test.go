package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureGit runs one git command in a fixture repository and returns its
// trimmed output. A failure fails the test, because a fixture that did not
// build proves nothing about the code under test.
func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// writeFixtureFile puts content at name inside the fixture repository.
func writeFixtureFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

// branched builds a repository in which the base branch moved after the branch
// under review left it, and returns the repository and the commit the two
// parted at.
//
// That shape is the whole point of the fixture. Only once main carries a commit
// feature never saw do a two-dot and a three-dot diff disagree, and only then
// does §3.4.1 say anything a test can fail.
func branched(t *testing.T) (dir, partedAt string) {
	t.Helper()
	dir = t.TempDir()
	fixtureGit(t, dir, "init", "--quiet", "--initial-branch=main")
	// The identity and the signing setting are repository-local. A machine
	// with no global user.email cannot commit at all — CI is such a
	// machine — and a test must not write a developer's own config.
	fixtureGit(t, dir, "config", "user.email", "fixture@cr.test")
	fixtureGit(t, dir, "config", "user.name", "cr fixture")
	fixtureGit(t, dir, "config", "commit.gpgsign", "false")

	writeFixtureFile(t, dir, "shared.txt", "a\nb\nc\n")
	fixtureGit(t, dir, "add", "shared.txt")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "shared")
	partedAt = fixtureGit(t, dir, "rev-parse", "HEAD")

	fixtureGit(t, dir, "checkout", "--quiet", "-b", "feature")
	writeFixtureFile(t, dir, "shared.txt", "a\nb\nc\nthe author's line\n")
	fixtureGit(t, dir, "commit", "--quiet", "-a", "-m", "the change under review")

	fixtureGit(t, dir, "checkout", "--quiet", "main")
	writeFixtureFile(t, dir, "somebody-else.txt", "landed on main after feature branched\n")
	fixtureGit(t, dir, "add", "somebody-else.txt")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "somebody else's commit")
	return dir, partedAt
}

// §3.4.1 takes the diff against the merge base at the current head, which is
// `git diff base...head` with three dots and never `base..head` with two. Two
// dots diffs the branch tips, so every commit that reached the base branch
// after the pull request branched appears in the output — reversed, as the
// author deleting code they never touched. The diff still looks plausible,
// which is what makes it the most expensive way this tool can be wrong.
func TestTheDiffExcludesWorkThatLandedOnTheBaseAfterBranching(t *testing.T) {
	dir, partedAt := branched(t)

	changed, err := DiffAgainstMergeBase(dir, "main", "feature")
	require.NoError(t, err)

	assert.Equal(t, partedAt, changed.MergeBase)
	assert.Contains(t, changed.Patch, "+the author's line")
	assert.NotContains(t, changed.Patch, "somebody-else.txt")
	assert.NotContains(t, changed.Patch, "landed on main after feature branched")
}

// §2.1.1 has the same state, head, and inputs give the same result. A
// configured external differ replaces git's output wholesale, and a developer
// who has one has it on for every repository they own, so cr pins it off
// rather than parsing whatever the machine happens to print.
func TestTheDiffIgnoresAConfiguredExternalDiffer(t *testing.T) {
	dir, _ := branched(t)
	fixtureGit(t, dir, "config", "diff.external", "echo an external differ ran")

	changed, err := DiffAgainstMergeBase(dir, "main", "feature")
	require.NoError(t, err)

	assert.Contains(t, changed.Patch, "+the author's line")
	assert.NotContains(t, changed.Patch, "an external differ ran")
}

// A run is given an allowlisted environment, not the ambient one: GIT_DIR and
// its neighbours would point the read at another repository altogether, and
// GIT_EXTERNAL_DIFF would replace the output the way the configuration file
// does. §2.1.1 makes that the inputs' business and not the shell's.
func TestAReadIgnoresTheAmbientGitEnvironment(t *testing.T) {
	dir, partedAt := branched(t)
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "not-a-repository"))
	t.Setenv("GIT_EXTERNAL_DIFF", "echo an external differ ran")

	changed, err := DiffAgainstMergeBase(dir, "main", "feature")
	require.NoError(t, err)

	assert.Equal(t, partedAt, changed.MergeBase)
	assert.Contains(t, changed.Patch, "+the author's line")
	assert.NotContains(t, changed.Patch, "an external differ ran")
}

// §3.1.3 fixes what an external command does when it refuses: the command's
// stderr reaches the user, and §11.2 codes it 3. git is one such command, so a
// failed read carries the command that ran, what git said about it, and the
// exec failure underneath, which tells a git that ran and refused from a git
// that never started.
func TestAFailedGitReadSurfacesItsStderr(t *testing.T) {
	dir, _ := branched(t)

	_, err := DiffAgainstMergeBase(dir, "main", "no-such-branch")

	var refused *CommandError
	require.ErrorAs(t, err, &refused)
	assert.Contains(t, refused.Stderr, "no-such-branch")
	assert.Contains(t, refused.Error(), refused.Stderr)
	assert.Contains(t, refused.Error(), "merge-base")
	var exited *exec.ExitError
	require.ErrorAs(t, err, &exited)

	silent := &CommandError{Args: []string{"merge-base"}, Err: errors.New("exit status 1")}
	assert.Equal(t, "git merge-base: exit status 1", silent.Error())
}
