// Package glob matches repository-relative paths against the globs cr's data
// files are written with.
//
// Two sections need the same matcher over the same kind of string. §2.4 selects
// a profile's test files with `tests.globs`, and §2.6 narrows a rule to `globs`
// and subtracts `exempt` from it. Both are repository-relative and
// slash-separated, as git writes a path in a diff header, and both are written
// by hand by the same person about the same tree — so `app/**` has to mean the
// same thing in a profile and in a rule, and one matcher is how that stays
// true rather than how it is asserted.
package glob

import (
	"path"
	"strings"
)

// doubleStar is the glob segment standing for any run of path segments,
// including none.
//
// It is why path.Match is not enough on its own: a `*` there never crosses a
// separator, so `tests/**/*Test.php` read through path.Match alone selects
// `tests/Feature/FooTest.php` and misses both `tests/FooTest.php` and
// `tests/Feature/Api/FooTest.php`. The shipped laravel-pest profile writes
// exactly that glob, so the wrong reading would attach a fraction of a Laravel
// suite while §4.4.1 said nothing about the rest.
const doubleStar = "**"

// Match reports whether target matches pattern, segment by segment, with `**`
// standing for any run of segments and every other segment matched by
// path.Match. Both are repository-relative and slash-separated.
//
// A pattern path.Match cannot compile — an unterminated `[` is the only way —
// matches nothing rather than aborting. Neither §2.4 nor §2.6 states a syntax
// rule for its globs to be held to, and inventing a rejection inside a matcher
// would be a normative decision taken where no one would look for one. Matching
// nothing is the safe direction of the two: cr attaches no file it never
// established belonged, and enforces no rule against a path the rule never
// named.
func Match(pattern, target string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(target, "/"))
}

// MatchAny reports whether target matches any of the patterns. No pattern
// matches nothing, which is the reading each caller then gives its own meaning:
// §2.4's absent `tests.globs` recognises no test file, and §2.6's empty `globs`
// means all source rather than none, so that one asks the question the other
// way round.
func MatchAny(patterns []string, target string) bool {
	for _, pattern := range patterns {
		if Match(pattern, target) {
			return true
		}
	}
	return false
}

// matchSegments matches the remaining pattern segments against the remaining
// path segments. `**` is the only one that consumes more or fewer than one
// segment, so it is the only branch that recurses: it tries every split of what
// is left, shortest first, which includes consuming nothing at all.
func matchSegments(patterns, segments []string) bool {
	for len(patterns) > 0 {
		if patterns[0] == doubleStar {
			for i := 0; i <= len(segments); i++ {
				if matchSegments(patterns[1:], segments[i:]) {
					return true
				}
			}
			return false
		}
		if len(segments) == 0 {
			return false
		}
		if ok, err := path.Match(patterns[0], segments[0]); err != nil || !ok {
			return false
		}
		patterns, segments = patterns[1:], segments[1:]
	}
	return len(segments) == 0
}
