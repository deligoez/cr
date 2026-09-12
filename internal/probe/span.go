package probe

import "github.com/deligoez/cr/internal/git"

// Span is the part of a record's anchor §6.2.2 reads when it binds a probe to
// that record: the path, the side, and the inclusive `start_line`..`line`
// range.
//
// It is a type of this package rather than finding.Anchor because the support
// decision lives here and internal/finding imports this package, not the other
// way round. The record's anchor is copied into it at the one place a grade's
// evidence is resolved.
type Span struct {
	// Path is the anchor's file.
	Path string
	// Side is the file version StartLine and Line are numbered in.
	Side git.Side
	// StartLine and Line bound the range, both ends inclusive.
	StartLine int
	Line      int
}

// Holds is §6.2.2's binding: a probe supports a record only when its `target`
// falls within the record's `anchor.start_line`..`anchor.line` range on the
// same path, so one experiment cannot grade a second finding elsewhere in the
// unit.
//
// A `LEFT` span holds nothing. §5.5 makes every probe target `RIGHT`, a line of
// the head, while a `LEFT` anchor numbers a removed line in the merge base, so
// the two numbers name lines of different file versions and a claim about
// deleted code is argued or cited, never probed. A target that does not parse
// is held by nothing either, which is the direction that can only lower a grade.
func (s Span) Holds(target string) bool {
	if s.Side != git.Right {
		return false
	}
	path, line, err := ParseTarget(target)
	if err != nil {
		return false
	}
	return path == s.Path && s.StartLine <= line && line <= s.Line
}
