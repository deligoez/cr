package profile

import "github.com/deligoez/cr/internal/glob"

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
//
// The matcher itself is internal/glob's, shared with §2.6's `globs` and
// `exempt`. `app/**` is written by the same hand about the same tree in a
// profile and in a rule, so it reads the same way in both by construction
// rather than by agreement.
func (p *Profile) IsTestFile(file string) bool {
	return glob.MatchAny(p.Tests.Globs, file)
}
