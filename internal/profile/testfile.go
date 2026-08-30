package profile

import (
	"path"
	"strings"
)

// doubleStar is the glob segment standing for any run of path segments,
// including none.
//
// `tests.globs` needs it and `path.Match` does not have it: a `*` there never
// crosses a separator, so `tests/**/*Test.php` read through `path.Match` alone
// selects `tests/Feature/FooTest.php` and misses both `tests/FooTest.php` and
// `tests/Feature/Api/FooTest.php`. The shipped laravel-pest profile writes
// exactly that glob, so the wrong reading would attach a fraction of a Laravel
// suite while §4.4.1 said nothing about the rest — evidence withheld from the
// agent's classification without either of them being told.
const doubleStar = "**"

// IsTestFile reports whether file names a test file of this profile, per §2.4's
// `tests.globs`. The path is repository-relative and slash-separated, as git
// writes it in a diff header.
//
// A profile with no `tests.globs` recognises no test file, and that state never
// reaches §4.4. §2.4 makes the field required whenever `tests.cmd` is present
// and §4.5.2 disables the test axis when `tests.cmd` is absent, so the two line
// up: a profile that can run tests can always name them, and one that cannot is
// switched off before the axis looks at anything. That is why this half of
// §4.4.1 reports no unavailable lens the way its symbol half does — the schema
// leaves it no state to be unavailable in.
func (p *Profile) IsTestFile(file string) bool {
	for _, glob := range p.Tests.Globs {
		if matchGlob(glob, file) {
			return true
		}
	}
	return false
}

// matchGlob matches one glob against one repository-relative path, segment by
// segment, with `**` standing for any run of segments and every other segment
// matched by path.Match.
//
// A glob path.Match cannot compile — an unterminated `[` is the only way — is
// treated as matching nothing rather than aborting the command. §2.4 states no
// syntax rule for `tests.globs` to be held to, and inventing a rejection inside
// a matcher would be a normative decision taken where no one would look for one.
func matchGlob(glob, target string) bool {
	return matchSegments(strings.Split(glob, "/"), strings.Split(target, "/"))
}

// matchSegments matches the remaining glob segments against the remaining path
// segments. `**` is the only one that consumes more or fewer than one segment,
// so it is the only branch that recurses: it tries every split of what is left,
// shortest first, which includes consuming nothing at all.
func matchSegments(globs, segments []string) bool {
	for len(globs) > 0 {
		if globs[0] == doubleStar {
			for i := 0; i <= len(segments); i++ {
				if matchSegments(globs[1:], segments[i:]) {
					return true
				}
			}
			return false
		}
		if len(segments) == 0 {
			return false
		}
		if ok, err := path.Match(globs[0], segments[0]); err != nil || !ok {
			return false
		}
		globs, segments = globs[1:], segments[1:]
	}
	return len(segments) == 0
}
