package intent

import (
	"iter"
	"strings"
)

// Paragraph is one blank-line separated block of the issue text.
type Paragraph struct {
	// StartLine and EndLine are the block's first and last lines, one-based
	// and inclusive, counted in the issue text as `cr brief` prints it.
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
	// Text is the block's lines exactly as the issue text holds them, so a
	// span copied from it occurs.
	Text string `json:"text"`
}

// Paragraphs is how many paragraphs the issue text has, and the ones no claim
// span overlaps.
type Paragraphs struct {
	// Total is every paragraph of the issue text.
	Total int `json:"total"`
	// Uncovered are the paragraphs no span of a claim drawn from the issue
	// text overlaps, in the order the text holds them.
	Uncovered []Paragraph `json:"uncovered"`
}

// UncoveredParagraphs splits the issue text into paragraphs and returns the
// ones no claim's span overlaps.
//
// It is text arithmetic and nothing more. A paragraph is a maximal run of lines
// that are neither blank nor a separator line of §3.1.5, where a line is split
// at LF and is blank when it holds nothing but SPACE, TAB and CR — the lines
// §1.4's steps 2 and 3 empty. §3.3.4 excludes the separator lines, and a
// separator breaks a run as a blank line does: it is cr's own line rather than
// a line of any source's text, so a paragraph that ran through one would be a
// paragraph no single claim could ever cover per §3.3.1. A
// paragraph is covered when some occurrence of some claim's span overlaps one
// of its bytes, however little of it that is. Nothing is ranked, and no
// paragraph is called more important than another: which of them states a
// requirement is the agent's judgement, and this only says where no claim was
// drawn from.
//
// Only claims drawn from the issue text are read. A claim sourced from a note
// has a note's body for its span, which is not text of this issue. A span is
// looked for cleaned, for the reason SpanTexts.issueHolds gives: issue is the
// cleaned text, and a span recorded before cleaning may hold a U+00A0 it no
// longer has.
func UncoveredParagraphs(issue string, claims iter.Seq[Claim]) Paragraphs {
	blocks := paragraphsOf(issue)
	covered := make([]bool, len(blocks))
	for claim := range claims {
		if claim.Source == ClaimFromNote {
			continue
		}
		span := Clean(claim.Span)
		if span == "" {
			continue
		}
		for at := 0; ; {
			found := strings.Index(issue[at:], span)
			if found < 0 {
				break
			}
			start := at + found
			markOverlaps(blocks, covered, start, start+len(span))
			at = start + 1
		}
	}
	report := Paragraphs{Total: len(blocks), Uncovered: make([]Paragraph, 0)}
	for i := range blocks {
		if !covered[i] {
			report.Uncovered = append(report.Uncovered, blocks[i].Paragraph)
		}
	}
	return report
}

// block is a Paragraph together with the byte range it spans in the issue
// text, end exclusive.
type block struct {
	Paragraph
	start, end int
}

// markOverlaps marks every block the byte range [start, end) overlaps.
func markOverlaps(blocks []block, covered []bool, start, end int) {
	for i := range blocks {
		if start < blocks[i].end && blocks[i].start < end {
			covered[i] = true
		}
	}
}

// paragraphsOf splits the issue text into its paragraphs.
func paragraphsOf(issue string) []block {
	blocks := make([]block, 0)
	open := false
	offset := 0
	for number, line := range strings.Split(issue, "\n") {
		end := offset + len(line)
		switch {
		case strings.Trim(line, " \t\r") == "" || IsSeparator(line):
			open = false
		case open:
			last := &blocks[len(blocks)-1]
			last.EndLine = number + 1
			last.end = end
			last.Text = issue[last.start:end]
		default:
			blocks = append(blocks, block{
				Paragraph: Paragraph{StartLine: number + 1, EndLine: number + 1, Text: line},
				start:     offset, end: end,
			})
			open = true
		}
		offset = end + 1
	}
	return blocks
}
