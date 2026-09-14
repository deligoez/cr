package git

import (
	"fmt"
	"regexp"
	"slices"
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

// ParseSide reads one of §9.2's two side names, reporting whether the value is
// one of them.
//
// The set is closed here, beside the two constants, because Side is a defined
// string rather than a fenced type: every spelling the outside world produces
// is assignable to it. Two readers take one from outside — a `side` on an
// agent's NDJSON line per §9.2, and GitHub's `diffSide` on an ingested thread
// per §3.5.1 — and both have to close the same set, or the set is closed twice
// and can drift.
//
// A third value is not a harmless unknown. §6.1.2 resolves RIGHT against the
// head and LEFT against the merge base, so a side that is neither names a tree
// cr resolves nothing against; and §3.4.4 partitions a file's hunks by side, so
// it also matches no hunk — which is a thread that quietly attaches to no unit
// rather than an error anybody sees.
func ParseSide(value string) (Side, bool) {
	side := Side(value)
	return side, slices.Contains(sides, side)
}

// sides is §9.2's two values, the set ParseSide closes.
var sides = []Side{Right, Left}

// Sides returns that set, RIGHT first. The result is a copy, so a caller telling
// a person which sides there are (§4.6.2) names exactly the ones ParseSide reads.
func Sides() []Side {
	return append(make([]Side, 0, len(sides)), sides...)
}

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
	// Removed holds the merge-base numbers of every line the hunk removes,
	// in ascending order, whichever side Changed is on. §9.2.1 lets a LEFT
	// anchor name only a removed line, and a modification's removed half is
	// in no Changed, while the context lines BaseStart and BaseLines cover
	// were removed by nobody.
	Removed []int
}

// HeadRange returns the hunk's head-side line range, both ends inclusive.
//
// §6.2.1 evaluates containment in head coordinates against "the head-side range
// of one of its hunks — for a hunk that adds no lines, its head-side insertion
// point". A hunk that adds lines answers with the range its header gives.
//
// A hunk that adds none — Side Left — answers with its insertion point: the
// head line its removal follows, and zero when the removal reaches the top of
// the file. The header's head range is not that point. It covers the context
// lines git keeps around the removal, and those lines were changed by nobody, so
// a location on one of them is outside the unit rather than inside it. A hunk
// whose removals are separated by context has one insertion point per run of
// removed lines, and its range runs from the first of them to the last.
//
// Every head line of such a hunk is a context line, and each maps to the next
// merge-base line that is not a removal, so the head line before the i-th
// removed line r is HeadStart plus the context lines ahead of r, less one. A
// hunk whose head side is empty — HeadLines is zero, which happens when the
// change deletes or empties the file — has only the point git wrote as the
// header's start.
func (h *Hunk) HeadRange() (start, end int) {
	if h.HeadLines == 0 {
		return h.HeadStart, h.HeadStart
	}
	if h.Side == Left && len(h.Removed) > 0 {
		last := len(h.Removed) - 1
		return h.insertionBefore(0), h.insertionBefore(last)
	}
	return h.HeadStart, h.HeadStart + h.HeadLines - 1
}

// insertionBefore is the head line that the i-th removed line of a hunk adding
// no lines follows: HeadStart plus the context lines ahead of it, less one.
func (h *Hunk) insertionBefore(i int) int {
	return h.HeadStart + (h.Removed[i] - h.BaseStart - i) - 1
}

// SideRange returns the range §3.4.6 records for the hunk, numbered on the side
// its changed lines are, both ends inclusive.
//
// §3.4.1 numbers a hunk that adds lines on the RIGHT and one that adds none on
// the LEFT, in the merge base. A hunk that adds lines answers with HeadRange. A
// hunk that adds none answers with its first and last removed lines, which a
// LEFT anchor counts in: not the header's merge-base range, whose context lines
// no LEFT anchor may name, and not a head line, which would put a reader's
// LEFT anchor on the wrong file version.
func (h *Hunk) SideRange() (start, end int) {
	if h.Side == Left && len(h.Removed) > 0 {
		return h.Removed[0], h.Removed[len(h.Removed)-1]
	}
	return h.HeadRange()
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
	hunks, _, err := parse(patch)
	return hunks, err
}

// HunkTexts returns the text of every hunk of a unified diff — its header line
// and every line of its body, context included, joined by LF — in the order
// ParseHunks returns the hunks, so the two slices index together.
//
// It exists for §4.6.1's prompt, which carries "the unit's hunks" to an agent
// who has to read them. A Hunk holds §3.4.1's changed lines and nothing else,
// so a modification arrives as its added half alone: the line it replaced is a
// removal, and §3.4.1 counts removals only in a hunk that adds nothing. A role
// judging a change it can only see the new half of is judging half a change.
//
// It is the same reading as ParseHunks rather than a second one. The two share
// parse, so a hunk ends here exactly where ParseHunks ends it — by its header's
// counts, not by where the following lines stop looking like content — and a
// patch ParseHunks refuses is refused here with the same error.
func HunkTexts(patch string) ([]string, error) {
	_, texts, err := parse(patch)
	return texts, err
}

// parse reads a unified diff into its hunks and, index for index, each hunk's
// text. ParseHunks and HunkTexts are its two views.
//
//nolint:funlen // measured 2026-09-11 at 51 statements; refactor to clear, never raise the limit
func parse(patch string) ([]Hunk, []string, error) {
	hunks := make([]Hunk, 0)
	texts := make([]string, 0)
	var body []string
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
				return nil, nil, fmt.Errorf("diff line %d: a hunk holds no empty line, because a blank context line keeps its leading space", n+1)
			}
			body = append(body, line)
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
				return nil, nil, fmt.Errorf("diff line %d: %q is not a hunk line", n+1, line)
			}
			if baseLeft <= 0 && headLeft <= 0 {
				current.Side, current.Changed, current.Removed = Right, added, lineNumbers(removed)
				if len(added) == 0 {
					current.Side, current.Changed = Left, removed
				}
				hunks, open = append(hunks, current), false
				texts = append(texts, strings.Join(body, "\n"))
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
				return nil, nil, fmt.Errorf("diff line %d: %q is not a hunk header", n+1, line)
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
				return nil, nil, fmt.Errorf("diff line %d: %q covers no line on either side", n+1, line)
			}
			added, removed = make([]ChangedLine, 0), make([]ChangedLine, 0)
			body = []string{line}
			baseLine, headLine = current.BaseStart, current.HeadStart
			baseLeft, headLeft = current.BaseLines, current.HeadLines
			open = true
		}
	}
	if open {
		return nil, nil, fmt.Errorf("the patch ends inside the hunk at %s:%d", current.Path, current.HeadStart)
	}
	return hunks, texts, nil
}

// lineNumbers is the numbers of lines, in their order.
func lineNumbers(lines []ChangedLine) []int {
	numbers := make([]int, 0, len(lines))
	for _, line := range lines {
		numbers = append(numbers, line.Line)
	}
	return numbers
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
