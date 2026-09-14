package suggestion

import (
	"fmt"
	"strings"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
)

// IndentationWarning is §8.2.3's disclosure: the suggestion's first line and
// the first line it replaces are indented differently.
//
// It is a warning and never a refusal, which is the whole of §8.2.3's first
// sentence. cr cannot tell a suggestion that means to re-indent a block from
// one whose author copied it out of a differently indented file, and guessing
// either way would be cr forming an opinion about the code — P5's line. So both
// lines are put in front of the reviewer, who can edit the block or leave it,
// and leaving it posts what it says.
type IndentationWarning struct {
	// Record is the id of the record carrying the suggestion.
	Record string
	// Suggested is the suggestion's first line, whole.
	Suggested string
	// Replaced is the first line the suggestion would replace, whole.
	Replaced string
}

// String shows both lines, quoted, because the difference is whitespace and an
// unquoted rendering of it is invisible on a terminal — a tab and four spaces
// occupy the same width and are not the same indentation.
func (w *IndentationWarning) String() string {
	return fmt.Sprintf(
		"record %s: the suggestion's first line and the line it replaces are indented differently, "+
			"and §8.2.3 has cr infer nothing about indentation. Leave the block in place to post it "+
			"as written, or edit it.\n  suggestion: %q\n  replaces:   %q",
		w.Record, w.Suggested, w.Replaced)
}

// WarnIndentation is §8.2.3 over one record, returning nil when there is
// nothing to warn about.
//
// Nothing is warned about in three cases, and each is an absence rather than a
// judgement. A record with no suggestion replaces nothing; a record §8.2.1
// refuses outright is the business of Validate, whose refusal says more than a
// warning would; and a first replaced line cr cannot read is a line cr has no
// indentation to compare against.
//
// texts is git.HunkTexts over the patch hunks were parsed from, index for index.
// A hunk holds §3.4.1's changed lines alone, while Validate admits any line of
// a hunk's head range, and that range carries the diff's context lines too. A
// suggestion replacing a context line replaces a head line all the same, so its
// text is read from the hunk's own body, where the context lines are.
//
// The comparison is of the leading whitespace alone and of the first line
// alone, in those words, because that is what §8.2.3 names. A suggestion whose
// later lines are indented differently is a suggestion about a block, and the
// first line is where GitHub anchors the replacement.
func WarnIndentation(record *finding.Finding, hunks []git.Hunk, texts []string) *IndentationWarning {
	if record.Suggestion == "" || !Placeable(&record.Anchor, hunks) {
		return nil
	}
	replaced, found := replacedLine(&record.Anchor, hunks, texts)
	if !found {
		return nil
	}
	suggested, _, _ := strings.Cut(record.Suggestion, "\n")
	if indentOf(suggested) == indentOf(replaced) {
		return nil
	}
	return &IndentationWarning{Record: record.ID, Suggested: suggested, Replaced: replaced}
}

// replacedLine returns the text of the first line the anchored range replaces,
// reporting whether the diff carries it. The hunk's text is read for a changed
// line and a context line alike, so the two cannot be told apart by which of
// them the warning can see.
func replacedLine(anchor *finding.Anchor, hunks []git.Hunk, texts []string) (string, bool) {
	for at := range hunks {
		if hunks[at].Path != anchor.Path || at >= len(texts) {
			continue
		}
		if text, found := headLine(&hunks[at], texts[at], anchor.StartLine); found {
			return text, true
		}
	}
	return "", false
}

// headLine returns head line n as a hunk's text carries it, without the diff's
// leading marker, reporting whether the hunk covers it.
//
// The text is the hunk's header and then its body, as git.HunkTexts gives it.
// A context line and an added line each stand for one head line, counted from
// the header's head start; a removed line and git's "\ No newline" annotation
// stand for none.
func headLine(hunk *git.Hunk, text string, n int) (string, bool) {
	_, body, _ := strings.Cut(text, "\n")
	at := hunk.HeadStart
	for line := range strings.SplitSeq(body, "\n") {
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "+") {
			continue
		}
		if at == n {
			return line[1:], true
		}
		at++
	}
	return "", false
}

// indentOf returns a line's leading whitespace, which is what §8.2.3 compares.
func indentOf(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}
