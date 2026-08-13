package git

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Side names the file version a line number is counted in.
//
// §9.2 fixes the two values: RIGHT anchors a line in the head, LEFT a removed
// line in the merge base. Every changed line carries its side rather than
// leaving it implicit in which number was stored, because §3.4.4 partitions a
// file's hunks by it and a unit never mixes sides, §6.1.2 resolves a RIGHT
// anchor against the head and a LEFT one against the merge base, and §6.2.2
// makes a probe target always RIGHT so a LEFT-anchored record can never reach
// probed. A number that has lost its side names a line in each of two file
// versions, and a finding pointing at the wrong one is the expensive kind of
// wrong.
type Side string

const (
	// Right numbers a line in the head.
	Right Side = "RIGHT"
	// Left numbers a line in the merge base.
	Left Side = "LEFT"
)

// ChangedLine is one line §3.4.1 counts as changed.
type ChangedLine struct {
	// Side is the file version Line is numbered in.
	Side Side
	// Line is the line's number on Side.
	Line int
	// Text is the line's content, without the diff's leading marker.
	Text string
}

// Hunk is one @@ block of a diff, with the changed lines §3.4.1 gives it.
//
// The two coordinate pairs come from the hunk header and cover context as well
// as change: base is the merge-base side, head the side at the current head. A
// count of zero is a position between two lines rather than a line — git writes
// the empty range's start as the line it follows, so `@@ -41,0 +42,3 @@` adds
// three lines after base line 41 and has no pre-image line of its own.
type Hunk struct {
	// Path is the file the hunk belongs to, without the a/ or b/ prefix. It
	// is the head-side path, falling back to the merge-base path for a file
	// the change deletes.
	Path string
	// BaseStart is the hunk's first line in the merge base.
	BaseStart int
	// BaseLines is how many merge-base lines the hunk covers.
	BaseLines int
	// HeadStart is the hunk's first line at the head.
	HeadStart int
	// HeadLines is how many head lines the hunk covers.
	HeadLines int
	// Side is the side Changed is numbered on: RIGHT for a hunk that adds
	// any line, LEFT for one that adds none. §3.4.4 partitions on it.
	Side Side
	// Changed holds the hunk's changed lines in ascending line order.
	Changed []ChangedLine
}

// HeadRange returns the hunk's head-side line range, both ends inclusive.
//
// §6.2.1 evaluates containment in head coordinates against "the head-side range
// of one of its hunks — for a hunk that adds no lines, its head-side insertion
// point". A hunk whose head side is empty — HeadLines is zero, which happens
// when the change deletes or empties the file — has no range to fall inside, so
// both ends are the insertion point git wrote as the header's start: the line
// the removal follows, and zero when the removal reaches the top of the file.
// Whether a hunk adds any line at all is Side, which is Left for exactly the
// hunk §3.4.1 describes in merge-base coordinates.
func (h Hunk) HeadRange() (start, end int) {
	if h.HeadLines == 0 {
		return h.HeadStart, h.HeadStart
	}
	return h.HeadStart, h.HeadStart + h.HeadLines - 1
}

// hunkHeader matches a hunk header, whose trailing section heading is not part
// of either range. Either count is omitted when it is 1.
var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// ParseHunks reads the hunks and the changed lines of a unified diff.
//
// A changed line is §3.4.1's: an added or modified RIGHT-side line, and for a
// hunk that adds none, its removed LEFT-side lines. A unified diff has no
// marker for a modification — an edited line is a removal and an addition
// standing together — so a hunk's added lines are exactly its added and
// modified head-side lines, and the removed lines of a hunk that has none are
// what is left to describe it.
//
// The parser and the flags of diffArgs are one contract. The unified context,
// the inter-hunk context and the pinned algorithm decide where a hunk begins
// and ends; --no-ext-diff, --no-textconv and --no-color decide that there is a
// unified diff to read at all; and the pinned a/ and b/ prefixes are what the
// file headers are read against. Loosening one half without the other is a
// defect even when both still compile.
//
// A hunk ends where its header's counts run out and not where the following
// lines stop looking like content, so a diff of a patch file — whose added
// lines begin with +++, --- and @@ — is read as the content it is.
func ParseHunks(patch string) ([]Hunk, error) {
	hunks := make([]Hunk, 0)
	var (
		basePath, headPath string
		open               bool
		current            Hunk
		added, removed     []ChangedLine
		baseLine, headLine int
		baseLeft, headLeft int
	)

	for n, line := range strings.Split(strings.TrimSuffix(patch, "\n"), "\n") {
		if open {
			if line == "" {
				return nil, fmt.Errorf("diff line %d: a hunk holds no empty line, because a blank context line keeps its leading space", n+1)
			}
			switch line[0] {
			case ' ':
				baseLine, headLine = baseLine+1, headLine+1
				baseLeft, headLeft = baseLeft-1, headLeft-1
			case '+':
				added = append(added, ChangedLine{Side: Right, Line: headLine, Text: line[1:]})
				headLine, headLeft = headLine+1, headLeft-1
			case '-':
				removed = append(removed, ChangedLine{Side: Left, Line: baseLine, Text: line[1:]})
				baseLine, baseLeft = baseLine+1, baseLeft-1
			case '\\':
				// "\ No newline at end of file" annotates the line
				// above it and belongs to neither version, so it
				// consumes nothing.
			default:
				return nil, fmt.Errorf("diff line %d: %q is not a hunk line", n+1, line)
			}
			if baseLeft <= 0 && headLeft <= 0 {
				current.Side, current.Changed = Right, added
				if len(added) == 0 {
					current.Side, current.Changed = Left, removed
				}
				hunks, open = append(hunks, current), false
			}
			continue
		}

		switch {
		case strings.HasPrefix(line, "--- "):
			basePath = headerPath(line[4:], "a/")
		case strings.HasPrefix(line, "+++ "):
			headPath = headerPath(line[4:], "b/")
		case strings.HasPrefix(line, "@@ "):
			fields := hunkHeader.FindStringSubmatch(line)
			if fields == nil {
				return nil, fmt.Errorf("diff line %d: %q is not a hunk header", n+1, line)
			}
			current = Hunk{
				Path:      headPath,
				BaseStart: headerNumber(fields[1]),
				BaseLines: headerCount(fields[2]),
				HeadStart: headerNumber(fields[3]),
				HeadLines: headerCount(fields[4]),
			}
			if current.Path == "" {
				current.Path = basePath
			}
			if current.BaseLines == 0 && current.HeadLines == 0 {
				return nil, fmt.Errorf("diff line %d: %q covers no line on either side", n+1, line)
			}
			added, removed = make([]ChangedLine, 0), make([]ChangedLine, 0)
			baseLine, headLine = current.BaseStart, current.HeadStart
			baseLeft, headLeft = current.BaseLines, current.HeadLines
			open = true
		}
	}
	if open {
		return nil, fmt.Errorf("the patch ends inside the hunk at %s:%d", current.Path, current.HeadStart)
	}
	return hunks, nil
}

// headerPath reads the path out of a --- or +++ file header.
//
// git writes the path behind the prefix diffArgs pins, appends a tab when the
// path holds a space, and C-quotes the whole field when the path holds a
// character it will not write raw. /dev/null stands for the side the file does
// not exist on, and is reported as no path at all.
func headerPath(field, prefix string) string {
	field, _, _ = strings.Cut(field, "\t")
	if strings.HasPrefix(field, `"`) {
		if unquoted, err := strconv.Unquote(field); err == nil {
			field = unquoted
		}
	}
	if field == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(field, prefix)
}

// headerNumber reads a hunk-header field the pattern has already proved to be
// digits.
func headerNumber(field string) int {
	parsed, _ := strconv.Atoi(field)
	return parsed
}

// headerCount reads a hunk-header count, which the header omits when it is 1.
func headerCount(field string) int {
	if field == "" {
		return 1
	}
	return headerNumber(field)
}
