package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// committed builds a repository whose single commit holds the files given,
// keyed by repository-relative path.
func committed(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := fixtureRepo(t)
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "the commit under review")
	return dir
}

// §6.2.3 resolves a citation against the current head, and a head is a commit.
// The distinction is invisible on a clean checkout and decides the answer on a
// dirty one, which is exactly the case §2.1.1 legislates: a command is
// reproducible given the same state directory, the same head SHA, and the same
// inputs, and an editor's unsaved buffer is none of the three.
//
// The fixture moves the worktree away from the commit in both directions a
// wrong read would show up in — a line whose content changed, and a line that
// exists only in the worktree — so a reader that opened the file on disk would
// hash the wrong text and would count a line the head does not have as in range.
func TestAFileIsReadFromTheCommitAndNotFromTheWorktree(t *testing.T) {
	dir := committed(t, map[string]string{"app.go": "package app\n\nfunc Retry() {}\n"})
	writeFixtureFile(t, dir, "app.go", "package app\n\nfunc Retry() { unsaved() }\nfunc Added() {}\n")

	lines, exists, err := FileAtRevision(dir, "HEAD", "app.go")

	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, []string{"package app", "", "func Retry() {}"}, lines,
		"§2.1.1: the same head must give the same lines, whatever the checkout has become since")
}

