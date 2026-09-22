package profile

import _ "embed"

// laravelPestID is the id, and therefore the file stem, of the profile §2.4.5
// requires v0.1 to ship for a Laravel repository whose suite is Pest.
const laravelPestID = "laravel-pest"

// laravelPest is the shipped profile file, embedded rather than built from a
// struct literal so what cr writes is the file itself, byte for byte. §2.5.2
// asks exactly that of the built-in roles, and a profile written from a literal
// could not answer it: the bytes would be whatever the JSON encoder chose that
// day, not the reviewed file in this repository.
//
//go:embed builtin/laravel-pest.json
var laravelPest string

// genericID is the id, and therefore the file stem, of the second profile
// §2.4.5 requires v0.1 to ship: the one for a repository no language-specific
// profile fits.
const genericID = "generic"

// generic is the shipped fallback profile, embedded like laravelPest. Its
// emptiness is what it has to say rather than an unfinished state, because a
// profile that knows no language cannot honestly claim a marker file, a test
// runner, or a symbol language. It declares no `match.files`, so §2.4.3 keeps
// it out of automatic selection and only a config naming it ever applies it; no
// `tests.cmd`, so §4.5.2 disables the test axis; and no `symbols.lang`, so the
// reinvention half of §4.3.1 is marked unavailable. §4.5.4 is what carries all
// of that to the reader instead of letting it pass as a quiet skip.
//
// `axes` still enables all four ids of §1.5, because §2.4 makes it the default
// enabled state and §4.5.1 checks the prerequisites separately. Writing
// `"test": false` would attribute the dead test axis to configuration rather
// than to the missing `tests.cmd` §4.5.2 names, and would leave a user who adds
// a runner to their own copy with the axis still switched off somewhere else.
//
//go:embed builtin/generic.json
var generic string

// goID is the id, and therefore the file stem, of the profile §2.4.5 ships for
// a Go module.
const goID = "go"

// goProfile is the shipped Go profile, embedded like laravelPest.
//
// Three of its fields are what `go test` needs and Pest does not, each measured
// against go1.27.1 on 2026-09-22 (testdata/go holds the captured output):
//
//   - `tests.count_mode` is `occurrences`, because `go test -v` prints one
//     `--- PASS:` or `--- FAIL:` line per test and no recap line with a number
//     on it, so there is nothing for §5.2.1's sum to read.
//   - The patterns are anchored at `^`, because a subtest's line is indented
//     under its parent's: the anchor counts the top-level tests, the ones
//     `-run` selects. SKIP is not counted, for the reason laravel-pest does
//     not count skipped tests: a run of nothing but skips must not read as a
//     run where nothing failed.
//   - `tests.paths_default` is `./...`, because a bare `go test` tests the
//     package in the current directory and nothing else, and `tests.paths_arg`
//     prefixes `./` so a path names a package rather than an import path.
//
// `tests.probe_path_template` uses `<target-dir>`, because a Go test is
// compiled into the package it tests (§5.4.2).
//
//go:embed builtin/go.json
var goProfile string

// Builtins returns the profile files cr ships, keyed by profile id, for `cr
// init` to write into the profiles directory of §2.2. The values are file
// contents, not parsed profiles, because writing them out is the whole purpose
// and a round trip through Profile would not reproduce them.
//
// The map is rebuilt per call so no caller can edit the shipped set.
func Builtins() map[string]string {
	return map[string]string{
		laravelPestID: laravelPest,
		goID:          goProfile,
		genericID:     generic,
	}
}
