// Package unit forms the review units of §3.4 out of a pull request's hunks.
package unit

import "github.com/deligoez/cr/internal/profile"

// SymbolIndex is what §3.4's symbol handling reads: which files cr has a
// symbol index for, and which symbol encloses a line of one of them.
//
// §4.3.1's head symbol index is what will implement this. §3.4.3 asks the
// narrower of the two questions — whether there is an index for a file at all
// — and §3.4.4's symbol branch asks the other, so the later index slots in
// behind both without a caller of Detectable or Clusters changing.
//
// Indexed returns a bool and not a (bool, error). §3.4.3 requires clustering
// to fall through "without reporting an error", and the way to keep an error
// out of the fallthrough is to leave it nowhere to come from. A file cr cannot
// parse, a language cr has no parser for, and a file that holds no symbols are
// one answer here, because clustering does the same thing with all three.
//
// Enclosing is asked only about head-side lines, because §4.3.1 builds the
// index over the head. A LEFT number is a line of the merge base, where the
// same number names a different line, and a symbol read out of the wrong file
// version would gather hunks under the name of code that does not contain
// them. §3.4.4 partitions by side before it clusters, and that partition is
// what makes the restriction expressible at all: the LEFT side has no index in
// its coordinate space, so it is not detectable there and falls through to
// adjacency per §3.4.3.
type SymbolIndex interface {
	// Indexed reports whether cr could build a symbol index for path.
	Indexed(path string) bool
	// Enclosing names the symbol enclosing the head-side line at path, and
	// reports false when no symbol encloses it.
	Enclosing(path string, line int) (string, bool)
}

// Detectable answers §3.4.3 for one file: an enclosing symbol is detectable
// only when the profile declares symbols.lang and cr can build a symbol index
// for the file. A nil index is cr having none — which is every run until
// §4.3.1's index exists — and is a negative answer, not a missing one.
//
// The same absent symbols.lang obliges §3.4.3 and §4.3.1 to opposite things,
// and this is the side allowed to be quiet. Clustering that cannot see a
// symbol still produces units: adjacency groups the same hunks into a
// different shape, every changed line still lands in exactly one unit, and
// every unit is still reviewed. Nothing the reader would have been told about
// went missing, so §3.4.3 lets the fallthrough be silent. §4.3.1 has no second
// branch to fall to: with no index there are no candidate symbols, the
// reinvention search produces nothing, and a silent nothing reads as "cr
// looked and found no reinvention" — an assertion cr never made. That is why
// profile.MissingProfile carries ReinventionLens in its Unavailable list while
// nothing here reports anything at all.
//
// So this returns one bit and no error, and a caller is left holding nothing
// it could hand to §4.5's report. A detectability answer cannot be mistaken
// for an unavailability one because it has no shape an unavailability report
// could be read out of.
func Detectable(p *profile.Profile, index SymbolIndex, path string) bool {
	if p.Symbols.Lang == "" {
		return false
	}
	if index == nil {
		return false
	}
	return index.Indexed(path)
}
