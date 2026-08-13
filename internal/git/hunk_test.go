package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eachShape builds a repository whose branch under review carries one hunk of
// each shape §3.4.1 distinguishes: a hunk that only adds, a hunk that modifies,
// and a hunk that adds nothing and only removes.
func eachShape(t *testing.T) string {
	t.Helper()
	dir := fixtureRepo(t)
	writeFixtureFile(t, dir, "added.txt", "alpha\nbeta\ngamma\n")
	writeFixtureFile(t, dir, "modified.txt", "one\ntwo\nthree\n")
	writeFixtureFile(t, dir, "removed.txt", "1\n2\n3\n4\n5\n6\n7\n8\n")
	fixtureGit(t, dir, "add", "-A")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "the code before the change")

	fixtureGit(t, dir, "checkout", "--quiet", "-b", "feature")
	writeFixtureFile(t, dir, "added.txt", "alpha\nbeta\ngamma\nthe added line\n")
	writeFixtureFile(t, dir, "modified.txt", "one\nthe modified line\nthree\n")
	writeFixtureFile(t, dir, "removed.txt", "1\n2\n3\n4\n6\n7\n8\n")
	fixtureGit(t, dir, "commit", "--quiet", "-a", "-m", "one hunk of each shape")
	return dir
}

// §3.4.1 makes a changed line an added or modified RIGHT-side line, and for a
// hunk that adds none, its removed LEFT-side lines — and the line numbers
// RIGHT-side for the first case and LEFT-side for the second. The side is not
// decoration: a hunk is normally described at the head and a delete-only hunk in
// merge-base coordinates, so a number read on the wrong side names a line in a
// file version nobody was talking about.
func TestChangedLinesCarryTheSideTheyAreNumberedOn(t *testing.T) {
	dir := eachShape(t)

	diff, err := DiffAgainstMergeBase(dir, "main", "feature")
	require.NoError(t, err)
	hunks, err := ParseHunks(diff.Patch)
	require.NoError(t, err)
	require.Len(t, hunks, 3)

	byPath := make(map[string]Hunk, len(hunks))
	for _, hunk := range hunks {
		byPath[hunk.Path] = hunk
	}

	// An added line is a changed line, numbered at the head.
	adds := byPath["added.txt"]
	assert.Equal(t, Right, adds.Side)
	assert.Equal(t, []ChangedLine{{Side: Right, Line: 4, Text: "the added line"}}, adds.Changed)

	// A unified diff has no marker for a modification: an edited line is a
	// removal and an addition standing together. The head-side half is the
	// changed line, and the pre-image the author replaced is not a second
	// one — counting it would double the size of every edit.
	modifies := byPath["modified.txt"]
	assert.Equal(t, Right, modifies.Side)
	assert.Equal(t, []ChangedLine{{Side: Right, Line: 2, Text: "the modified line"}}, modifies.Changed)

	// A hunk that adds none has nothing at the head to point at, so it is
	// its removed lines, numbered in the merge base. Line 5 is where "5"
	// stood before the change; at the head that number is another line.
	removes := byPath["removed.txt"]
	assert.Equal(t, Left, removes.Side)
	assert.Equal(t, []ChangedLine{{Side: Left, Line: 5, Text: "5"}}, removes.Changed)
}
