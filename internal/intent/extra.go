package intent

import "strings"

// The two halves of §3.1.5's separator line, which stands between one part of
// the issue text and the next.
//
// It is fixed rather than configurable for the reason the question label of
// §8.1.4 is: §3.3.1 holds every span to one part, so the line is what says
// where a part ends, and a project that could reword it could make a span
// checked against a text it was never drawn from.
const (
	separatorPrefix = "--- cr intent file: "
	separatorSuffix = " ---"
)

// Separator is §3.1.5's separator line for one extra intent file, naming the
// path as it was given.
//
// The path is written as given rather than resolved. §3.3's `file` row names
// the same string, so a claim drawn from an extra file names the path the
// reader of the issue text saw above it; resolving one side and not the other
// would leave a claim naming a file the text never mentions.
func Separator(path string) string {
	return separatorPrefix + path + separatorSuffix
}

// IsSeparator reports whether one line of the issue text is a separator line
// of §3.1.5.
//
// §3.3.4 excludes such a line from every paragraph, so the question is asked
// of the text rather than of the paths this run was given: a round that reads
// its issue text back out of state (`cr status`) has the text and not the
// flags. A trailing CR is trimmed first, because §3.3.4 already reads CR as
// padding when it decides whether a line is blank.
func IsSeparator(line string) bool {
	line = strings.TrimSuffix(line, "\r")
	return len(line) >= len(separatorPrefix)+len(separatorSuffix) &&
		strings.HasPrefix(line, separatorPrefix) &&
		strings.HasSuffix(line, separatorSuffix)
}

// Part is one part of the issue text of §3.1.5: the text the tracker or
// `--intent-file` produced, or one extra intent file's text.
//
// §3.3.1 checks a claim's span against exactly one part, so Text is the part's
// own region of the issue text rather than the bytes the source produced. The
// two differ by at most the newline joinParts adds to keep a separator line on
// a line of its own, and the difference is the one that matters: a span copied
// out of the issue text a reader was shown occurs in the region and need not
// occur in the file.
type Part struct {
	// File is the extra intent file's path as `--intent-extra` or
	// `intent.extra_files` gave it, and is empty for the part §3.1.1
	// through §3.1.4 produced.
	File string `json:"file,omitempty"`
	// Text is the part's region of the issue text.
	Text string `json:"text"`
}

// ExtraFiles is §3.1.5's list of extra intent files: the `--intent-extra`
// paths in the order given, then the `intent.extra_files` paths, each path
// once.
//
// The deduplication is by the path as given, which is what the section says of
// a path both sources name. It is applied within each source too: two
// identical paths would otherwise write the same separator line twice, and
// §3.3.1 resolves a `source: file` claim by that path, so the claim would name
// two parts and the check would have to pick one.
func ExtraFiles(flag, configured []string) []string {
	paths := make([]string, 0, len(flag)+len(configured))
	seen := make(map[string]bool, len(flag)+len(configured))
	for _, source := range [][]string{flag, configured} {
		for _, path := range source {
			if seen[path] {
				continue
			}
			seen[path] = true
			paths = append(paths, path)
		}
	}
	return paths
}

// join builds §3.1.5's issue text out of the text one of §3.1.1 through §3.1.4
// produced and the extra intent files' texts, each under a separator line
// naming its path.
//
// A newline is added before a separator line when the text so far does not end
// in one, because §3.1.5 makes the separator a line and a line begins where the
// previous one ended.
func join(base string, extras []Part) string {
	text := base
	for _, extra := range extras {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += Separator(extra.File) + "\n" + extra.Text
	}
	return text
}

// Parts splits an issue text into §3.1.5's parts: the text before the first
// separator line, then one part per separator line, each holding the path that
// line names and the text from the line after it to the next separator line or
// the end.
//
// The partition is read out of the text rather than carried beside it, and
// that is what makes §3.3.1 answerable by whoever holds the text. `cr status`
// reads the round's issue text back out of state with no flags to consult, and
// a partition kept as state could disagree with the document it describes —
// while this one is a property of the bytes a claim's span is searched in.
//
// Every part's text is a slice of the issue text, so a span copied out of what
// a reader was shown occurs in the part it was copied from. A text with no
// separator line is one part: the whole of it.
func Parts(issue string) []Part {
	parts := make([]Part, 0, 1)
	current, start, offset := Part{}, 0, 0
	for line := range strings.SplitSeq(issue, "\n") {
		end := offset + len(line)
		if IsSeparator(line) {
			current.Text = issue[start:offset]
			parts = append(parts, current)
			current, start = Part{File: separatorPath(line)}, min(end+1, len(issue))
		}
		offset = end + 1
	}
	current.Text = issue[start:]
	return append(parts, current)
}

// separatorPath is the path a separator line names, as given.
func separatorPath(line string) string {
	line = strings.TrimSuffix(line, "\r")
	return strings.TrimSuffix(strings.TrimPrefix(line, separatorPrefix), separatorSuffix)
}
