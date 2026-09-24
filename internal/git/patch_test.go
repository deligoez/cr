package git

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What `git diff` really writes, headers and all. §5.3.1 has an agent supply
// the mutation, and the shape it will supply is whatever `git diff` handed it —
// so the lines the parser is expected to walk past are part of the contract, not
// noise a hand-written fixture can leave out.
const realDiff = `diff --git a/src/Order.php b/src/Order.php
index 1a2b3c4..5d6e7f8 100644
--- a/src/Order.php
+++ b/src/Order.php
@@ -31,7 +31,7 @@ final class Order
     public function shipping(): float
     {
         if ($this->subtotal() >= self::FREE_SHIPPING_AT) {
-            return 0.0;
+            return 999.0;
         }
 
         return self::SHIPPING;
`

// ParsePatch reads a real diff, keeping each hunk's body whole.
//
// The body is the whole point of this parser existing beside ParseHunks: §3.4.1
// wants the changed lines and throws the context away, and applying a patch
// needs exactly what was thrown away — the context is what says the patch
// matches the file in front of it.
func TestParsePatchKeepsEachHunkWhole(t *testing.T) {
	files, err := ParsePatch(realDiff)
	require.NoError(t, err)
	require.Len(t, files, 1)

	assert.Equal(t, "src/Order.php", files[0].Path)
	assert.Equal(t, "src/Order.php", files[0].BasePath)
	assert.Equal(t, "src/Order.php", files[0].HeadPath)
	require.Len(t, files[0].Hunks, 1)

	hunk := files[0].Hunks[0]
	assert.Equal(t, 31, hunk.BaseStart)
	assert.Equal(t, 7, hunk.BaseLines)
	assert.Equal(t, 31, hunk.HeadStart)
	assert.Equal(t, 7, hunk.HeadLines)
	assert.Equal(t, []string{
		"     public function shipping(): float",
		"     {",
		"         if ($this->subtotal() >= self::FREE_SHIPPING_AT) {",
		"-            return 0.0;",
		"+            return 999.0;",
		"         }",
		" ",
		"         return self::SHIPPING;",
	}, hunk.Body, "the `diff --git` and `index` lines are walked past, and the body is kept as written")
}

// A patch over several files and several hunks is read as all of them.
func TestParsePatchReadsEveryFileAndHunk(t *testing.T) {
	files, err := ParsePatch(
		"--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-one\n+ONE\n" +
			"@@ -5 +5 @@\n-five\n+FIVE\n" +
			"--- a/b.txt\n+++ b/b.txt\n@@ -2,2 +2,2 @@\n three\n-four\n+FOUR\n")
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Len(t, files[0].Hunks, 2)
	assert.Len(t, files[1].Hunks, 1)
	assert.Equal(t, "b.txt", files[1].Path)
}

// A file the patch creates or deletes is read as one, with the missing side
// reported as no path rather than as /dev/null.
func TestParsePatchReportsTheSideAFileIsAbsentOn(t *testing.T) {
	files, err := ParsePatch(
		"--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1 @@\n+one\n" +
			"--- a/gone.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-one\n")
	require.NoError(t, err)
	require.Len(t, files, 2)

	assert.Equal(t, "new.txt", files[0].Path)
	assert.Empty(t, files[0].BasePath, "a created file has no pre-image path")
	assert.Equal(t, "gone.txt", files[1].Path,
		"a deleted file is named by the side it exists on")
	assert.Empty(t, files[1].HeadPath)
}

// Each file names the +++ header that opened it, one-based, so a refusal of
// the path the header names can point the reader at the line to correct.
func TestParsePatchNamesTheLineOfEachFileHeader(t *testing.T) {
	files, err := ParsePatch(
		"--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-one\n+ONE\n" +
			"--- a/b.txt\n+++ b/b.txt\n@@ -2 +2 @@\n-two\n+TWO\n")
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Equal(t, 2, files[0].HeaderLine)
	assert.Equal(t, 7, files[1].HeaderLine)
}

// A patch that is not a patch is refused, rather than half-read.
//
// §5.3.4's first rung answers a patch that does not apply, and this is the step
// before it: a diff cr cannot even read is not an experiment cr can perform, and
// guessing at one would put a mutation somewhere nobody wrote.
func TestParsePatchRefusesWhatItCannotRead(t *testing.T) {
	for name, broken := range map[string]struct {
		patch   string
		problem string
		// line is the one-based line of the patch the refusal names,
		// and zero for the one refusal that is about the patch as a
		// whole. The number is asserted rather than the message alone,
		// because it is what the reader opens their own patch at.
		line int
	}{
		"a hunk before any file header": {
			patch:   "@@ -1 +1 @@\n-one\n+ONE\n",
			problem: "arrives before any --- and +++ file header",
			line:    1,
		},
		"a hunk header that is not one": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ nonsense @@\n",
			problem: "is not a hunk header",
			line:    3,
		},
		"a header covering no line at all": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ -0,0 +0,0 @@\n",
			problem: "covers no line on either side",
			line:    3,
		},
		"a body line with no marker": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\nno marker\n",
			problem: "is not a hunk line",
			line:    4,
		},
		"an empty line inside a hunk": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ -1,2 +1,2 @@\n one\n\n",
			problem: "a hunk holds no empty line",
			line:    5,
		},
		"a patch that stops mid-hunk": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ -1,3 +1,3 @@\n one\n",
			problem: "ends inside a hunk",
		},
	} {
		t.Run(name, func(t *testing.T) {
			files, err := ParsePatch(broken.patch)
			require.Error(t, err)
			assert.Nil(t, files)
			assert.Contains(t, err.Error(), broken.problem)
			if broken.line == 0 {
				assert.NotContains(t, err.Error(), "patch line",
					"a refusal about the whole patch names no line in it")
				return
			}
			assert.Contains(t, err.Error(), fmt.Sprintf("patch line %d:", broken.line),
				"the refusal names the line the reader has to open")
		})
	}
}

// A line outside every hunk that is no file header is refused where it stands,
// and only in a patch that names a file. Output with no file header at all is
// what a configured diff.external writes in a diff's place, and it comes back
// as no files and no error, so the caller's no-hunk refusal can name the flag
// that fixes it instead of this line.
func TestParsePatchRefusesAStrayLineOnlyInAPatchThatNamesAFile(t *testing.T) {
	files, err := ParsePatch("--- a/a.txt\n+++ b/a.txt\nrename from a.txt\n@@ -1 +1 @@\n-one\n+ONE\n")
	var malformed *MalformedPatchError
	require.ErrorAs(t, err, &malformed)
	assert.Nil(t, files)
	assert.Equal(t, 3, malformed.Line)
	assert.Contains(t, malformed.Problem, "rename, copy or mode header")

	files, err = ParsePatch("Only in left: a.txt\nFiles differ\n")
	require.NoError(t, err)
	assert.Empty(t, files)
}

// Apply rewrites the file the way the patch says, and only there.
func TestApplyRewritesWhatThePatchNames(t *testing.T) {
	for name, tc := range map[string]struct {
		patch   string
		content string
		want    string
	}{
		"a replaced line": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ -2,3 +2,3 @@\n one\n-two\n+TWO\n three\n",
			content: "zero\none\ntwo\nthree\n",
			want:    "zero\none\nTWO\nthree\n",
		},
		"two hunks in one file": {
			patch: "--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-one\n+ONE\n" +
				"@@ -4 +4 @@\n-four\n+FOUR\n",
			content: "one\ntwo\nthree\nfour\n",
			want:    "ONE\ntwo\nthree\nFOUR\n",
		},
		"an insertion between two lines": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ -1,2 +1,3 @@\n one\n+ONE AND A HALF\n two\n",
			content: "one\ntwo\n",
			want:    "one\nONE AND A HALF\ntwo\n",
		},
		"a zero-context insertion": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ -1,0 +2 @@\n+ONE AND A HALF\n",
			content: "one\ntwo\n",
			want:    "one\nONE AND A HALF\ntwo\n",
		},
		// An append lands one past the last line, which is the one
		// position that is both inside the patch and outside the file.
		"an append at the end of the file": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ -2,0 +3 @@\n+three\n",
			content: "one\ntwo\n",
			want:    "one\ntwo\nthree\n",
		},
		"a removal": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ -1,3 +1,2 @@\n one\n-two\n three\n",
			content: "one\ntwo\nthree\n",
			want:    "one\nthree\n",
		},
		"a file emptied entirely": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ -1,2 +0,0 @@\n-one\n-two\n",
			content: "one\ntwo\n",
			want:    "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			files, err := ParsePatch(tc.patch)
			require.NoError(t, err)
			require.Len(t, files, 1)

			applied, err := files[0].Apply(tc.content)
			require.NoError(t, err)
			assert.Equal(t, tc.want, applied)
		})
	}
}

// The final newline is the last hunk's to change, and nothing else's.
//
// `\ No newline at end of file` annotates the line above it, and which side it
// describes is which marker that line carries. A patch that reads a file
// without a final newline and leaves it without one has to be applied without
// adding one, or the mutation cr ran differs from the mutation the agent wrote.
func TestApplyCarriesTheFinalNewlineThePatchDescribes(t *testing.T) {
	for name, tc := range map[string]struct {
		patch   string
		content string
		want    string
	}{
		"a post-image that loses its newline": {
			patch: "--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-one\n" +
				"+ONE\n\\ No newline at end of file\n",
			content: "one\n",
			want:    "ONE",
		},
		"a pre-image that had none and a post-image that does": {
			patch: "--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-one\n" +
				"\\ No newline at end of file\n+ONE\n",
			content: "one",
			want:    "ONE\n",
		},
		"an appended line that ends the file without one": {
			patch: "--- a/a.txt\n+++ b/a.txt\n@@ -1,0 +2 @@\n+two\n" +
				"\\ No newline at end of file\n",
			content: "one\n",
			want:    "one\ntwo",
		},
		"a hunk that does not reach the end leaves it alone": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-one\n+ONE\n",
			content: "one\ntwo",
			want:    "ONE\ntwo",
		},
	} {
		t.Run(name, func(t *testing.T) {
			files, err := ParsePatch(tc.patch)
			require.NoError(t, err)

			applied, err := files[0].Apply(tc.content)
			require.NoError(t, err)
			assert.Equal(t, tc.want, applied)
		})
	}
}

// A hunk that does not match the file it addresses is refused, and the refusal
// names both texts.
//
// There is no offset search and no fuzz, which git's own `apply` offers and cr
// does not want: a hunk that matched three lines away matched something the
// agent did not write the patch against, and §5.3.2 has the evidence chain run
// on the patch cr executed. The reader is told what was expected and what is
// there, because a stale patch and a drifted sandbox are opposite fixes.
func TestApplyRefusesAHunkThatDoesNotMatch(t *testing.T) {
	files, err := ParsePatch("--- a/a.txt\n+++ b/a.txt\n@@ -1,2 +1,2 @@\n one\n-TWO\n+two\n")
	require.NoError(t, err)

	applied, err := files[0].Apply("one\ntwo\n")
	var refused *ApplyError
	require.ErrorAs(t, err, &refused)
	assert.Empty(t, applied)
	assert.Equal(t, "a.txt", refused.Path)
	assert.Equal(t, 2, refused.Line)
	assert.Equal(t, "TWO", refused.Want)
	assert.Equal(t, "two", refused.Found)
	assert.Contains(t, refused.Error(), "a.txt line 2: the hunk does not match the file")
	assert.Contains(t, refused.Error(), `expected "TWO" and the file holds "two"`,
		"a stale patch and a drifted sandbox are opposite fixes, so both texts are named")
}

// A hunk aimed past the end of the file is refused too, and says so without a
// line number, because there is no line there to name.
func TestApplyRefusesAHunkTheFileIsTooShortFor(t *testing.T) {
	for name, tc := range map[string]struct{ patch, content, problem string }{
		"a hunk starting past the end": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ -9,1 +9,1 @@\n-nine\n+NINE\n",
			content: "one\n",
			problem: "the hunk at 9 addresses a line the file does not hold",
		},
		"a hunk running past the end": {
			patch:   "--- a/a.txt\n+++ b/a.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n",
			content: "one\ntwo\n",
			problem: "the hunk at 1 reaches past the end of the file",
		},
		"two hunks in descending order": {
			patch: "--- a/a.txt\n+++ b/a.txt\n@@ -3 +3 @@\n-three\n+THREE\n" +
				"@@ -1 +1 @@\n-one\n+ONE\n",
			content: "one\ntwo\nthree\n",
			problem: "the hunk at 1 addresses a line the file does not hold",
		},
	} {
		t.Run(name, func(t *testing.T) {
			files, err := ParsePatch(tc.patch)
			require.NoError(t, err)

			applied, err := files[0].Apply(tc.content)
			var refused *ApplyError
			require.ErrorAs(t, err, &refused)
			assert.Empty(t, applied)
			assert.Zero(t, refused.Line, "there is no line to name")
			assert.Equal(t, "a.txt: "+tc.problem, refused.Error(),
				"a refusal with no line to name says so by naming none")
		})
	}
}
