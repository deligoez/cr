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

// The header shapes that hide an off-by-one, and the content shapes that look
// like structure. A hunk header omits a count when it is 1 and writes 0 for a
// position between two lines rather than a line, so the first and last lines of
// a file, a new file, and a deleted one each say something different about where
// a changed line sits.
func TestTheAwkwardShapesOfAHunk(t *testing.T) {
	for _, shape := range []struct {
		name  string
		patch string
		want  []Hunk
	}{
		{
			name: "a count the header omitted is one line",
			patch: `--- a/tiny.txt
+++ b/tiny.txt
@@ -1 +1,2 @@
 one
+two
`,
			want: []Hunk{{
				Path: "tiny.txt", BaseStart: 1, BaseLines: 1, HeadStart: 1, HeadLines: 2,
				Side:    Right,
				Changed: []ChangedLine{{Side: Right, Line: 2, Text: "two"}},
			}},
		},
		{
			name: "a change at the first line of the file",
			patch: `--- a/keep.txt
+++ b/keep.txt
@@ -1,3 +1,4 @@
+the new first line
 a
 b
 c
`,
			want: []Hunk{{
				Path: "keep.txt", BaseStart: 1, BaseLines: 3, HeadStart: 1, HeadLines: 4,
				Side:    Right,
				Changed: []ChangedLine{{Side: Right, Line: 1, Text: "the new first line"}},
			}},
		},
		{
			name: "a new file has no pre-image at all",
			patch: `--- /dev/null
+++ b/new.txt
@@ -0,0 +1,2 @@
+first
+second
`,
			want: []Hunk{{
				Path: "new.txt", BaseStart: 0, BaseLines: 0, HeadStart: 1, HeadLines: 2,
				Side: Right,
				Changed: []ChangedLine{
					{Side: Right, Line: 1, Text: "first"},
					{Side: Right, Line: 2, Text: "second"},
				},
			}},
		},
		{
			name: "an insertion point names the line it follows",
			patch: `--- a/f.txt
+++ b/f.txt
@@ -41,0 +42,3 @@ func f() {
+one
+two
+three
`,
			want: []Hunk{{
				Path: "f.txt", BaseStart: 41, BaseLines: 0, HeadStart: 42, HeadLines: 3,
				Side: Right,
				Changed: []ChangedLine{
					{Side: Right, Line: 42, Text: "one"},
					{Side: Right, Line: 43, Text: "two"},
					{Side: Right, Line: 44, Text: "three"},
				},
			}},
		},
		{
			name: "a deleted file leaves no head-side line and keeps its old path",
			patch: `--- a/gone.txt
+++ /dev/null
@@ -1,3 +0,0 @@
-x
-y
-z
`,
			want: []Hunk{{
				Path: "gone.txt", BaseStart: 1, BaseLines: 3, HeadStart: 0, HeadLines: 0,
				Side: Left,
				Changed: []ChangedLine{
					{Side: Left, Line: 1, Text: "x"},
					{Side: Left, Line: 2, Text: "y"},
					{Side: Left, Line: 3, Text: "z"},
				},
			}},
		},
		{
			name: "a missing final newline is a line of neither version",
			patch: `--- a/tail.txt
+++ b/tail.txt
@@ -1 +1 @@
-the last line
\ No newline at end of file
+the last line, edited
\ No newline at end of file
`,
			want: []Hunk{{
				Path: "tail.txt", BaseStart: 1, BaseLines: 1, HeadStart: 1, HeadLines: 1,
				Side:    Right,
				Changed: []ChangedLine{{Side: Right, Line: 1, Text: "the last line, edited"}},
			}},
		},
		{
			name: "a patch under review is content, not structure",
			patch: `--- a/example.patch
+++ b/example.patch
@@ -1,2 +1,6 @@
 context
+--- a/inner.txt
++++ b/inner.txt
+@@ -1 +1 @@
+-old
 tail
`,
			want: []Hunk{{
				Path: "example.patch", BaseStart: 1, BaseLines: 2, HeadStart: 1, HeadLines: 6,
				Side: Right,
				Changed: []ChangedLine{
					{Side: Right, Line: 2, Text: "--- a/inner.txt"},
					{Side: Right, Line: 3, Text: "+++ b/inner.txt"},
					{Side: Right, Line: 4, Text: "@@ -1 +1 @@"},
					{Side: Right, Line: 5, Text: "-old"},
				},
			}},
		},
		{
			name:  "a path holding a space carries a trailing tab",
			patch: "--- a/with space.txt\t\n+++ b/with space.txt\t\n@@ -1 +1 @@\n-old\n+new\n",
			want: []Hunk{{
				Path: "with space.txt", BaseStart: 1, BaseLines: 1, HeadStart: 1, HeadLines: 1,
				Side:    Right,
				Changed: []ChangedLine{{Side: Right, Line: 1, Text: "new"}},
			}},
		},
		{
			name: "a path git would not write raw is unquoted",
			patch: `--- "a/we\"ird.txt"
+++ "b/we\"ird.txt"
@@ -1 +1 @@
-old
+new
`,
			want: []Hunk{{
				Path: `we"ird.txt`, BaseStart: 1, BaseLines: 1, HeadStart: 1, HeadLines: 1,
				Side:    Right,
				Changed: []ChangedLine{{Side: Right, Line: 1, Text: "new"}},
			}},
		},
		{
			name:  "a diff of nothing holds no hunk",
			patch: "",
			want:  []Hunk{},
		},
		{
			name: "a mode change alone holds no hunk",
			patch: `diff --git a/script.sh b/script.sh
old mode 100644
new mode 100755
`,
			want: []Hunk{},
		},
	} {
		t.Run(shape.name, func(t *testing.T) {
			hunks, err := ParseHunks(shape.patch)
			require.NoError(t, err)
			assert.Equal(t, shape.want, hunks)
		})
	}
}
