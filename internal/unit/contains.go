package unit

// Contains is §6.2.1's containment predicate: a `path:line` is **inside** a
// unit when `path` is one of that unit's file paths and `line` falls within the
// head-side range of one of its hunks — for a hunk that adds no lines, its
// head-side insertion point.
//
// Every number is head-side, which is §6.2.1's "entirely in head coordinates"
// rather than a simplification here. Range is recorded from git.Hunk.HeadRange,
// so a hunk that adds no lines is already stored as its insertion point with
// Start equal to End, and this needs no case of its own for it. No side is
// compared: the unit's own Side says which tree its changed lines are numbered
// in, and reading it here would answer in two coordinate systems the question
// §6.2.1 answers in one.
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
	for _, hunk := range u.HunkRanges {
		if line >= hunk.Start && line <= hunk.End {
			return true
		}
	}
	return false
}
