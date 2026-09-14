package suggestion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
)

// indentedDiff adds one tab-indented line, which is the changed line the cases
// below compare a suggestion's indentation against. Its second hunk's added line
// is unindented, so a fixture can also pair a suggestion with a line that
// agrees, and its two context lines, head lines 11 and 40, are unindented too.
const indentedDiff = `--- a/app/Models/Order.php
+++ b/app/Models/Order.php
@@ -10,2 +10,2 @@
-	$removed = 2;
+	$added = 2;
 $tail = 3;
@@ -40,1 +40,2 @@
 $keep = 1;
+$more = 2;
`

// replacing is a record whose suggestion replaces the anchored line with text.
func replacing(start, end int, text string) *finding.Finding {
	anchor := right("app/Models/Order.php", start, end)
	record := suggesting(&anchor)
	record.Suggestion = text
	return record
}

// warnOn is WarnIndentation over indentedDiff, parsed the way a run parses it:
// the hunks and, index for index, their texts.
func warnOn(t *testing.T, record *finding.Finding) *IndentationWarning {
	t.Helper()
	texts, err := git.HunkTexts(indentedDiff)
	require.NoError(t, err)
	return WarnIndentation(record, record.Suggestion, hunksOf(t, indentedDiff), texts)
}

// §8.2.3: a suggestion indented unlike the line it replaces is warned about,
// and both lines are shown.
//
// Quoting is what makes "show both" mean anything here. The difference is
// whitespace, and a tab and four spaces are the same width on a terminal, so a
// warning that printed the lines plain would show the reviewer two lines that
// look identical and tell them they differ.
func TestASuggestionIndentedUnlikeItsLineIsWarnedAbout(t *testing.T) {
	record := replacing(10, 10, "    $added = 3;")

	warning := warnOn(t, record)

	require.NotNil(t, warning)
	assert.Equal(t, "f1", warning.Record)
	assert.Equal(t, "    $added = 3;", warning.Suggested)
	assert.Equal(t, "\t$added = 2;", warning.Replaced)
	assert.Contains(t, warning.String(), `"    $added = 3;"`)
	assert.Contains(t, warning.String(), `"\t$added = 2;"`)
	assert.Contains(t, warning.String(), "f1")
}

// §8.2.3 is asked of the suggestion that will be sent. A reviewer who re-indents
// the fence in draft.md changes what reaches the author, so the sent text is the
// one compared, and a stored suggestion that agrees with its line says nothing
// about it.
func TestTheSentSuggestionIsTheOneCompared(t *testing.T) {
	record := replacing(10, 10, "\t$added = 3;")
	texts, err := git.HunkTexts(indentedDiff)
	require.NoError(t, err)

	warning := WarnIndentation(record, "    $added = 3;\n    $second = 4;", hunksOf(t, indentedDiff), texts)

	require.NotNil(t, warning)
	assert.Equal(t, IndentationWarning{Record: "f1", Suggested: "    $added = 3;", Replaced: "\t$added = 2;"}, *warning)
	assert.Nil(t, WarnIndentation(record, "", hunksOf(t, indentedDiff), texts),
		"a body whose fence the reviewer removed sends no suggestion to compare")
}

// §8.2.3 over a context line: Validate admits every line of a hunk's head
// range, and git's three lines of context are in that range, so a suggestion
// replacing one of them replaces a head line and its first line is compared
// against that line's own text.
//
// Both context lines of the fixture are asked about because they sit on either
// side of the change. Line 11 follows a removed and an added line, and a
// reading that counted the removal as a head line would compare against
// `\t$added = 2;` instead; line 40 is the second hunk's first line, which a
// reading that started counting one line late would never reach.
func TestASuggestionReindentingAContextLineIsWarnedAbout(t *testing.T) {
	for _, c := range []struct {
		name     string
		line     int
		replaced string
	}{
		{name: "a context line after the change", line: 11, replaced: "$tail = 3;"},
		{name: "a context line opening a hunk", line: 40, replaced: "$keep = 1;"},
	} {
		t.Run(c.name, func(t *testing.T) {
			record := replacing(c.line, c.line, "    $replacement = 4;")
			require.NoError(t, Validate(record, hunksOf(t, indentedDiff)),
				"the fixture's context line is one §8.2 lets a suggestion replace")

			warning := warnOn(t, record)

			require.NotNil(t, warning)
			assert.Equal(t, IndentationWarning{
				Record: "f1", Suggested: "    $replacement = 4;", Replaced: c.replaced,
			}, *warning)
		})
	}
}

// §8.2.3 is a warning and not a refusal: the same record passes §8.2's
// validation, so nothing about the mismatch blocks the post. A reviewer who
// leaves the block in place posts it as written.
func TestAnIndentationMismatchDoesNotBlockPosting(t *testing.T) {
	hunks := hunksOf(t, indentedDiff)
	record := replacing(10, 10, "    $added = 3;")

	require.NotNil(t, warnOn(t, record))
	assert.NoError(t, Validate(record, hunks),
		"§8.2.4 blocks on §8.2.1 and §8.2.2, and indentation is neither")
}

// The cases §8.2.3 has nothing to say about. Each is an absence rather than a
// judgement, and warning on any of them would be cr inferring the intent
// §8.2.3 forbids it to infer.
func TestNothingIsWarnedAboutWhenThereIsNothingToCompare(t *testing.T) {
	for _, c := range []struct {
		name   string
		record *finding.Finding
	}{
		{name: "the indentation agrees", record: replacing(10, 10, "\t$added = 3;")},
		{name: "the indentation of a context line agrees", record: replacing(11, 11, "$tail = 4;")},
		{name: "the record carries no suggestion", record: replacing(10, 10, "")},
		{name: "the diff carries no such line", record: replacing(400, 400, "    $added = 3;")},
		{name: "only a later line is indented differently",
			record: replacing(10, 10, "\t$added = 3;\n        $second = 4;")},
	} {
		t.Run(c.name, func(t *testing.T) {
			assert.Nil(t, warnOn(t, c.record))
		})
	}
}

// A hunk with no text beside it is a hunk whose lines cr cannot read, so it is
// warned about no more than a line the diff does not carry — and it is not an
// index out of range.
func TestAHunkWithoutItsTextIsWarnedAboutNothing(t *testing.T) {
	record := replacing(10, 10, "    $added = 3;")

	assert.Nil(t, WarnIndentation(record, record.Suggestion, hunksOf(t, indentedDiff), nil))
}
