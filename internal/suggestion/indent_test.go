package suggestion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// indentedDiff adds one tab-indented line, which is the line every case below
// compares a suggestion's indentation against. Its second hunk's added line is
// unindented, so a fixture can also pair a suggestion with a line that agrees.
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

// §8.2.3: a suggestion indented unlike the line it replaces is warned about,
// and both lines are shown.
//
// Quoting is what makes "show both" mean anything here. The difference is
// whitespace, and a tab and four spaces are the same width on a terminal, so a
// warning that printed the lines plain would show the reviewer two lines that
// look identical and tell them they differ.
func TestASuggestionIndentedUnlikeItsLineIsWarnedAbout(t *testing.T) {
	hunks := hunksOf(t, indentedDiff)
	record := replacing(10, 10, "    $added = 3;")

	warning := WarnIndentation(record, hunks)

	require.NotNil(t, warning)
	assert.Equal(t, "f1", warning.Record)
	assert.Equal(t, "    $added = 3;", warning.Suggested)
	assert.Equal(t, "\t$added = 2;", warning.Replaced)
	assert.Contains(t, warning.String(), `"    $added = 3;"`)
	assert.Contains(t, warning.String(), `"\t$added = 2;"`)
	assert.Contains(t, warning.String(), "f1")
}

// §8.2.3 is a warning and not a refusal: the same record passes §8.2's
// validation, so nothing about the mismatch blocks the post. A reviewer who
// leaves the block in place posts it as written.
func TestAnIndentationMismatchDoesNotBlockPosting(t *testing.T) {
	hunks := hunksOf(t, indentedDiff)
	record := replacing(10, 10, "    $added = 3;")

	require.NotNil(t, WarnIndentation(record, hunks))
	assert.NoError(t, Validate(record, hunks),
		"§8.2.4 blocks on §8.2.1 and §8.2.2, and indentation is neither")
}

// The cases §8.2.3 has nothing to say about. Each is an absence rather than a
// judgement, and warning on any of them would be cr inferring the intent
// §8.2.3 forbids it to infer.
func TestNothingIsWarnedAboutWhenThereIsNothingToCompare(t *testing.T) {
	hunks := hunksOf(t, indentedDiff)
	for _, c := range []struct {
		name   string
		record *finding.Finding
	}{
		{name: "the indentation agrees", record: replacing(10, 10, "\t$added = 3;")},
		{name: "the record carries no suggestion", record: replacing(10, 10, "")},
		{name: "the diff carries no such line", record: replacing(400, 400, "    $added = 3;")},
		{name: "only a later line is indented differently",
			record: replacing(10, 10, "\t$added = 3;\n        $second = 4;")},
	} {
		t.Run(c.name, func(t *testing.T) {
			assert.Nil(t, WarnIndentation(c.record, hunks))
		})
	}
}
