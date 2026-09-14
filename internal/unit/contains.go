package unit

// Contains is §6.2.1's containment predicate: a `path:line` is **inside** a
// unit when `path` is one of that unit's file paths and `line` falls within the
// head-side range of one of its hunks — for a hunk that adds no lines, its
// head-side insertion point.
//
// Every number is head-side, which is §6.2.1's "entirely in head coordinates"
// rather than a simplification here. HeadRanges is recorded from
// git.Hunk.HeadRange, so a hunk that adds no lines is already stored as its
// insertion point, and this needs no case of its own for it. No side is
// compared: the unit's own Side says which tree its changed lines are numbered
// in, and reading it here would answer in two coordinate systems the question
// §6.2.1 answers in one.
//
// A unit record without HeadRanges was written before the field existed, when
// HunkRanges was head-side for every unit, so its HunkRanges are read in its
// place. An empty HeadRanges is a unit with no hunks and is not replaced.
//
// The path is an equality because the set §6.2.1's "one of that unit's file
// paths" ranges over always holds exactly one element — see Unit.Path, where
// the reason is §3.4.4's own.
//
// It lives with the unit rather than with the grade, and satisfies
// finding.Containment without importing internal/finding, the way
// probe.GapUnmappable satisfies finding.HonestyDisclosure: the coordinates are
// this package's, and a predicate written where the grade is computed would be
// a second reading of what a hunk range means.
func (u *Unit) Contains(path string, line int) bool {
	if path != u.Path {
		return false
	}
	ranges := u.HeadRanges
	if ranges == nil {
		ranges = u.HunkRanges
	}
	for _, hunk := range ranges {
		if line >= hunk.Start && line <= hunk.End {
			return true
		}
	}
	return false
}
