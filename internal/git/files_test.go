package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// filesFixture builds a base commit and a head commit that between them touch
// a file of every shape ChangedFiles distinguishes, and returns the repository
// with the two revisions.
func filesFixture(t *testing.T) (dir, base, head string) {
	t.Helper()
	dir = fixtureRepo(t)
	writeFixtureFile(t, dir, "moved.txt", "one\ntwo\nthree\nfour\nfive\n")
	writeFixtureFile(t, dir, "script.sh", "echo hi\n")
	writeFixtureFile(t, dir, ".gitattributes", "*.pb.go linguist-generated\nschema.go linguist-generated=true\nplain.go -linguist-generated\n")
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "the base")
	base = fixtureGit(t, dir, "rev-parse", "HEAD")

	fixtureGit(t, dir, "mv", "moved.txt", "renamed.txt")
	fixtureGit(t, dir, "update-index", "--chmod=+x", "script.sh")
	writeFixtureFile(t, dir, "logo.png", "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	writeFixtureFile(t, dir, "edited.txt", "a line\n")
	fixtureGit(t, dir, "add", "logo.png", "edited.txt")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "the change under review")
	return dir, base, fixtureGit(t, dir, "rev-parse", "HEAD")
}

// A binary file is listed rather than dropped, which the patch alone cannot
// do: ParseHunks yields no hunk for a `Binary files … differ` entry. A rename
// reports its head path, and a rename or a mode change with no changed line is
// not Changed, because the patch holds no hunk for it.
func TestChangedFilesListsEveryFileIncludingTheBinaryOnes(t *testing.T) {
	dir, base, head := filesFixture(t)

	files, err := ChangedFiles(dir, base, head)

	require.NoError(t, err)
	assert.Equal(t, []ChangedFile{
		{Path: "edited.txt", Changed: true},
		{Path: "logo.png", Binary: true},
		{Path: "renamed.txt"},
		{Path: "script.sh"},
	}, files)
}

// A file is generated when the head's attributes set `linguist-generated` or
// set it to true, and not when they unset it or say nothing. The worktree's
// own .gitattributes is not consulted: §3.4.7's answer is the head's.
func TestGeneratedReadsTheHeadsAttributesAndNotTheWorktrees(t *testing.T) {
	dir, _, head := filesFixture(t)
	writeFixtureFile(t, dir, ".gitattributes", "edited.txt linguist-generated\n")

	generated, err := Generated(dir, head, []string{"api.pb.go", "schema.go", "plain.go", "edited.txt"})

	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"api.pb.go": true, "schema.go": true}, generated)
}

// A numstat listing git did not write is refused rather than read as fewer
// files.
func TestParseNumstatRefusesARecordItCannotRead(t *testing.T) {
	_, err := parseNumstat([]string{"no tabs here"})
	require.Error(t, err)

	_, err = parseNumstat([]string{"1\t0\t", "old.txt"})
	require.Error(t, err, "a rename record with no head path")
}
