package git

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every hunk's text is its header and its whole body, index for index with the
// hunk ParseHunks reads, and a modification carries both of its halves.
//
// The modified hunk is the case the text exists for. ParseHunks gives it one
// changed line — "the modified line", at the head — because §3.4.1 does not
// count the pre-image of an edit as a second one; a prompt built from changed
// lines alone would show a role the new line and never the line it replaced.
// The text keeps the removal, the addition, and the context the pinned
// --unified gives around them, so what the role reads is the change.
func TestEveryHunksTextIsItsHeaderAndItsWholeBody(t *testing.T) {
	dir := eachShape(t)
	diff, err := DiffAgainstMergeBase(dir, "main", "feature")
	require.NoError(t, err)

	hunks, err := ParseHunks(diff.Patch)
	require.NoError(t, err)
	texts, err := HunkTexts(diff.Patch)
	require.NoError(t, err)
	require.Len(t, texts, len(hunks), "the two readings index together")

	for i, text := range texts {
		lines := strings.Split(text, "\n")
		assert.True(t, strings.HasPrefix(lines[0], "@@ "), "hunk %d's text opens with its header", i)
		if hunks[i].Path != "modified.txt" {
			continue
		}
		assert.Equal(t, []string{"@@ -1,3 +1,3 @@", " one", "-two", "+the modified line", " three"}, lines,
			"both halves of the edit, with its context")
	}
}

// A patch ParseHunks refuses is refused by HunkTexts with the same error, so a
// prompt can never be built from a text the unit clustering never read.
func TestHunkTextsRefusesWhatParseHunksRefuses(t *testing.T) {
	patch := "--- a/f.txt\n+++ b/f.txt\n@@ -1,2 +1,2 @@\n one\n"
	_, refused := ParseHunks(patch)
	require.Error(t, refused)

	texts, err := HunkTexts(patch)
	require.EqualError(t, err, refused.Error())
	assert.Nil(t, texts)
}
