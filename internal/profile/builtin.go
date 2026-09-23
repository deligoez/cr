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

// typescriptID is the id, and therefore the file stem, of the profile §2.4.5
// ships for a TypeScript or JavaScript repository, Vue and React included.
const typescriptID = "typescript"

// typescriptProfile is the shipped TypeScript profile, embedded like
// laravelPest. Its runner is Vitest, measured at v5.0.1 on 2026-09-23
// (testdata/vitest holds the captured output):
//
//   - `tests.count_mode` is `occurrences` over the verbose reporter's
//     one-line-per-test ` ✓ ` and ` × `, not a sum over its `Tests` recap,
//     because the recap spells its counts `1 failed | 3 passed` and one
//     capture group cannot read both. A skipped test prints ` ↓ ` and is not
//     counted, for the reason laravel-pest and go do not count one.
//   - `npx --no` runs the repository's own Vitest and never installs one: a
//     repository without it exits 1 with no test line, which §5.2.1 reads as
//     undetermined rather than as a run of nothing. The `--` after it is
//     what hands every later argument to the runner: measured 2026-09-23,
//     npx takes `--verbose`, `--json` and `--version` as npm's own flags and
//     the runner never sees them.
//   - `match.files` names no `package.json`, which every JavaScript
//     repository carries, so a Jest repository is not a §2.4.2 tie with this
//     one on it (§2.4.5).
//   - `tests.paths_arg` is the path itself, which Vitest reads as a file
//     filter, and a path that selects no file exits 1 the same way.
//
// `tests.probe_path_template` uses `<target-dir>`, so a gap probe's test sits
// beside the module it imports, and `.test.ts` is inside Vitest's default
// include.
//
//go:embed builtin/typescript.json
var typescriptProfile string

// jestID is the id, and therefore the file stem, of the profile §2.4.5 ships
// for a JavaScript or TypeScript repository whose runner is Jest, the usual
// one for React and React Native.
const jestID = "jest"

// jestProfile is the shipped Jest profile, embedded like laravelPest. Its
// runner is Jest, measured at v30.5.2 on 2026-09-23 (testdata/jest holds the
// captured output):
//
//   - `tests.count_mode` is `sum` over `--json`'s `numPassedTests` and
//     `numFailedTests`, because Jest printed no line per test to count, not
//     even under `--verbose`, measured through a pipe as cr reads it. The keys are
//     read with their closing quote, so `numFailedTestSuites` matches neither.
//     A skipped or todo test is `numPendingTests` or `numTodoTests`, and is
//     not counted.
//   - A suite that throws on import reports both counts as zero and exits 1,
//     which §5.2.1 reads as undetermined; a path selecting no file prints no
//     JSON at all and exits 1, which is undetermined too.
//   - `match.files` names Jest's own configuration files and no
//     `package.json`, for the reason typescript's names none. A project that
//     configures Jest only inside `package.json` does not select this
//     profile and names it with the `profile` setting.
//
// `tests.probe_path_template` places a `.test.js` beside its target, which
// Jest's default `testMatch` collects whether or not the project transforms
// TypeScript.
//
//go:embed builtin/jest.json
var jestProfile string

// rustID is the id, and therefore the file stem, of the profile §2.4.5 ships
// for a Cargo package.
const rustID = "rust"

// rustProfile is the shipped Rust profile, embedded like laravelPest. Its
// runner is `cargo test`, measured at cargo 1.98.1 on 2026-09-23
// (testdata/cargo holds the captured output):
//
//   - `--no-fail-fast`, because without it cargo stops at the first test
//     binary that fails and the counts cover only the binaries before it.
//   - `tests.count_mode` is `occurrences` over libtest's `test <name> ... ok`
//     and `... FAILED` lines. Each binary's `test result:` recap gives
//     passed and failed as two numbers on one line, which one capture group
//     cannot read. An ignored test prints `... ignored` and is not counted.
//   - `tests.filter_flag` is `--`, which hands the expression to libtest as
//     its name filter. No `tests.paths_arg` is set, because cargo selects a
//     test target by name and not by file, so `--path` is refused.
//
// A build failure prints no test line and exits 101, which is undetermined.
// `tests.probe_path_template` is an integration test under `tests/`, the one
// place cargo compiles a new file without a `mod` declaration naming it.
//
//go:embed builtin/rust.json
var rustProfile string

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
		typescriptID:  typescriptProfile,
		jestID:        jestProfile,
		rustID:        rustProfile,
		genericID:     generic,
	}
}
