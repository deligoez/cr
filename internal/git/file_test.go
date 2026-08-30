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

// A line number resolves against the file's lines, so what counts as a line
// decides whether §6.2.3 calls a citation out of range.
//
// The final newline is a terminator rather than a separator: a file ending in
// one does not gain an empty last line, and a file holding nothing has no lines
// at all rather than one empty one. Both are off-by-one boundaries at the end of
// every file cr will ever read.
func TestTheLastLineIsCountedWhateverTheFinalNewline(t *testing.T) {
	for name, held := range map[string]struct {
		body  string
		lines []string
	}{
		"an empty file holds no line":              {"", nil},
		"a terminated line is one line":            {"only\n", []string{"only"}},
		"an unterminated line is one line as well": {"only", []string{"only"}},
		"a trailing blank line is a line":          {"only\n\n", []string{"only", ""}},
		"a file of one newline is one empty line":  {"\n", []string{""}},
		"two lines are two":                        {"first\nsecond\n", []string{"first", "second"}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := committed(t, map[string]string{"held.txt": held.body})

			lines, exists, err := FileAtRevision(dir, "HEAD", "held.txt")

			require.NoError(t, err)
			assert.True(t, exists)
			assert.Equal(t, held.lines, lines)
		})
	}
}

