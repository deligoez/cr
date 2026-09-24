package intent

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

// paragraphIssue has four paragraphs: one line, two wrapped lines padded the
// way jira pads them, a line after a whitespace-only separator, and a last one.
const paragraphIssue = "CR-1: orders ship free over 50 TL.\n" +
	"\n" +
	"The campaign counts sales in two windows:   \n" +
	"the first week and the last week.\n" +
	" \t\r\n" +
	"Refunds are excluded.\n" +
	"\n\n" +
	"Limit is 50 TL here.\n"

// Field feedback 2.9: a claim extraction skipped the paragraph stating the two
// counting windows, and nothing said so. The paragraphs no span overlaps are
// listed whole, with their lines, and nothing else is.
func TestUncoveredParagraphsAreTheOnesNoSpanOverlaps(t *testing.T) {
	claims := []Claim{
		{ID: "CR-1#c1", Source: ClaimFromDescription, Span: "orders ship free over 50 TL"},
		{ID: "CR-1#c2", Source: ClaimFromAcceptance, Span: "Refunds"},
		// A note claim's span is a note's body; that it also reads as
		// issue text covers nothing.
		{ID: "CR-1#c3", Source: ClaimFromNote, NoteID: "CR-1#n1", Span: "Limit is 50 TL here."},
	}

	assert.Equal(t, Paragraphs{Total: 4, Uncovered: []Paragraph{
		{StartLine: 3, EndLine: 4, Text: "The campaign counts sales in two windows:   \nthe first week and the last week."},
		{StartLine: 9, EndLine: 9, Text: "Limit is 50 TL here."},
	}}, UncoveredParagraphs(paragraphIssue, slices.Values(claims)))
}

// A span touching any byte of a paragraph covers it, including a span that runs
// across the line break inside a paragraph or across a blank line into the
// next, and every occurrence of a span counts.
func TestAnyOverlapCoversAParagraph(t *testing.T) {
	claims := []Claim{
		{ID: "CR-1#c1", Source: ClaimFromDescription, Span: "windows:   \nthe first"},
		{ID: "CR-1#c2", Source: ClaimFromDescription, Span: "50 TL"},
	}

	assert.Equal(t, Paragraphs{Total: 4, Uncovered: []Paragraph{
		{StartLine: 6, EndLine: 6, Text: "Refunds are excluded."},
	}}, UncoveredParagraphs(paragraphIssue, slices.Values(claims)))
}

// With no claim, every paragraph is uncovered; with no text, there is none.
func TestParagraphsWithoutClaimsOrText(t *testing.T) {
	report := UncoveredParagraphs(paragraphIssue, slices.Values([]Claim{}))
	assert.Equal(t, 4, report.Total)
	assert.Len(t, report.Uncovered, 4)
	assert.Equal(t, Paragraphs{Total: 0, Uncovered: []Paragraph{}},
		UncoveredParagraphs("", slices.Values([]Claim{})))
}

// A span recorded before cleaning is looked for cleaned, and only where the
// cleaned text reads the same: its U+00A0 form covers the paragraph it was
// drawn from and not a paragraph that says something else.
func TestASpanRecordedBeforeCleaningCoversOnlyItsOwnParagraph(t *testing.T) {
	cleaned := Clean(legacyIssue)
	claims := []Claim{legacyClaim(t)}

	assert.Equal(t, Paragraphs{Total: 2, Uncovered: []Paragraph{
		{StartLine: 3, EndLine: 4, Text: "An order under 50 TL pays shipping.\nView this issue on Jira"},
	}}, UncoveredParagraphs(cleaned, slices.Values(claims)))
}

// A paragraph is covered by a span that overlaps one of its bytes, and the line
// break after a paragraph or before the next is a byte of neither: a span that
// opens on the break below one paragraph, or closes on the break above the
// next, covers only the paragraph it reaches into. A span the text opens with
// covers the first paragraph like any other occurrence.
//
// gremlins found both edges open, and the first occurrence at offset zero:
// every fixture's spans began and ended inside a paragraph, and none began the
// text.
func TestASpanCoversOnlyTheParagraphsWhoseBytesItOverlaps(t *testing.T) {
	issue := "First.\n\nSecond.\n\nThird."
	claims := []Claim{
		{ID: "CR-1#c1", Source: ClaimFromDescription, Span: "\n\nSecond"},
		{ID: "CR-1#c2", Source: ClaimFromDescription, Span: "Second.\n\n"},
	}

	assert.Equal(t, Paragraphs{Total: 3, Uncovered: []Paragraph{
		{StartLine: 1, EndLine: 1, Text: "First."},
		{StartLine: 5, EndLine: 5, Text: "Third."},
	}}, UncoveredParagraphs(issue, slices.Values(claims)))

	opening := []Claim{{ID: "CR-1#c1", Source: ClaimFromDescription, Span: "First."}}
	assert.Equal(t, Paragraphs{Total: 3, Uncovered: []Paragraph{
		{StartLine: 3, EndLine: 3, Text: "Second."},
		{StartLine: 5, EndLine: 5, Text: "Third."},
	}}, UncoveredParagraphs(issue, slices.Values(opening)))
}
