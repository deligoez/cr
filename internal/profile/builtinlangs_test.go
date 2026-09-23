package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/glob"
)

// loadShipped parses one shipped profile with the loader a user's own file
// goes through.
func loadShipped(t *testing.T, id string) Profile {
	t.Helper()
	content, ships := Builtins()[id]
	require.True(t, ships, "cr ships %s", id)
	p, err := Parse(id+fileExt, []byte(content))
	require.NoError(t, err)
	return p
}

// §2.4.5: each language profile carries what the probe machinery needs to work
// at all, and each field is asserted at its value, for the reason go's are.
// Neither JavaScript profile names `package.json`, which every JavaScript
// repository carries and which would make the two a §2.4.2 tie everywhere.
func TestTheShippedTypescriptJestAndRustProfilesFillEveryFieldTheyNeed(t *testing.T) {
	ts := loadShipped(t, typescriptID)
	assert.Equal(t, []string{"tsconfig.json", "vite.config.ts", "vite.config.js", "vitest.config.ts", "vitest.config.js"},
		ts.Match.Files)
	assert.Equal(t, []string{"npx", "--no", "--", "vitest", "run", "--reporter=verbose"}, ts.Tests.Cmd)
	assert.Equal(t, "-t", ts.Tests.FilterFlag)
	assert.True(t, ts.CountsOccurrences(), "Vitest's recap has no one number to sum")

	js := loadShipped(t, jestID)
	assert.Equal(t, []string{"npx", "--no", "--", "jest", "--json"}, js.Tests.Cmd)
	assert.Equal(t, "-t", js.Tests.FilterFlag)
	assert.False(t, js.CountsOccurrences(), "Jest prints no line per test to count")

	for _, p := range []Profile{ts, js} {
		assert.NotContains(t, p.Match.Files, "package.json", p.ID)
		assert.Equal(t, "typescript", p.Symbols.Lang, p.ID)
		// The sandbox is a fresh worktree and node_modules is ignored, so
		// without the copy `npx --no` finds no runner (QA, 2026-09-23).
		assert.Equal(t, []string{"node_modules"}, p.Sandbox.Copy, p.ID)
	}
	assert.ElementsMatch(t, js.Match.Files, ts.Match.Unless,
		"§2.4.5: a file that selects jest keeps typescript out of automatic selection")

	rs := loadShipped(t, rustID)
	assert.Equal(t, []string{"Cargo.toml", "Cargo.lock"}, rs.Match.Files)
	assert.Equal(t, []string{"cargo", "test", "--no-fail-fast"}, rs.Tests.Cmd)
	assert.Equal(t, "--", rs.Tests.FilterFlag)
	assert.Equal(t, "rust", rs.Symbols.Lang)
	assert.True(t, rs.CountsOccurrences(), "cargo's recap gives two numbers on one line")

	for _, p := range []Profile{ts, js, rs} {
		assert.Equal(t, []string{}, p.Sandbox.Require, p.ID)
		for _, id := range axis.IDs() {
			assert.True(t, p.Axes[id], "%s: axes must enable %s", p.ID, id)
		}
	}
}

// §5.2.1's argv. Vitest and Jest take a path as a file filter, after the `--`
// that keeps npx from reading any of it; cargo takes a filter after `--` and
// has no path argument, so `--path` is refused rather than dropped.
func TestTheTypescriptJestAndRustArgvNarrowTheWayTheirRunnersDo(t *testing.T) {
	ts := loadShipped(t, typescriptID)
	scoped, err := ts.TestArgv(typescriptID+fileExt, "adds", []string{"src/math.test.ts"})
	require.NoError(t, err)
	assert.Equal(t, []string{"npx", "--no", "--", "vitest", "run", "--reporter=verbose", "-t", "adds", "src/math.test.ts"}, scoped)

	js := loadShipped(t, jestID)
	scoped, err = js.TestArgv(jestID+fileExt, "adds", []string{"src/math"})
	require.NoError(t, err)
	assert.Equal(t, []string{"npx", "--no", "--", "jest", "--json", "-t", "adds", "src/math"}, scoped)

	rs := loadShipped(t, rustID)
	filtered, err := rs.TestArgv(rustID+fileExt, "adds", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"cargo", "test", "--no-fail-fast", "--", "adds"}, filtered)

	_, err = rs.TestArgv(rustID+fileExt, "", []string{"src"})
	var unavailable *UnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, testPathsArgField, unavailable.Field)
	assert.True(t, strings.HasPrefix(unavailable.Hint(), "run without --path"),
		"cargo selects no test by file, so declaring tests.paths_arg is not the step (QA, 2026-09-23)")
}

// §2.4's table: each shipped profile's probe path matches its own test globs,
// so a placed gap probe is a file the runner collects.
func TestTheTypescriptJestAndRustProbePathsAreTestFiles(t *testing.T) {
	for id, want := range map[string]string{
		typescriptID: "src/cart/cr_probe_p3.test.ts",
		jestID:       "src/cart/cr_probe_p3.test.js",
		rustID:       "tests/cr_probe_p3.rs",
	} {
		p := loadShipped(t, id)
		placed := p.ProbePath("p3", "src/cart")
		assert.Equal(t, want, placed, id)
		assert.True(t, glob.MatchAny(p.Tests.Globs, placed), id)
	}
}

// The files under testdata/vitest are the real output of Vitest 5.0.1, under
// testdata/jest of Jest 30.5.2 and under testdata/cargo of cargo 1.98.1, all on
// darwin/arm64, captured 2026-09-23 with the exit status each run returned.
//
// The readings that matter: a skipped, todo or ignored test is not an executed
// one, so a run of nothing else reads zero; a filter matching nothing exits 0
// and is zero; and a run that loaded or built no test — a suite whose import
// failed, a path selecting no file, a crate that did not compile — exits
// non-zero having executed nothing, so it is undetermined rather than a run of
// nothing. Jest's suite that did not load is the case §5.2.1 changed for:
// its JSON says zero passed and zero failed outright.
func TestTheTypescriptJestAndRustPatternsCountCapturedOutput(t *testing.T) {
	cases := map[string]struct {
		profile, file    string
		exit             int
		executed, failed string
	}{
		"vitest, all passing":            {typescriptID, "vitest/all-passing.txt", 0, "2", "0"},
		"vitest, some failing":           {typescriptID, "vitest/some-failing.txt", 1, "4", "1"},
		"vitest, a filter matching none": {typescriptID, "vitest/no-tests-selected.txt", 0, "0", "0"},
		"vitest, a suite that did not load": {
			typescriptID, "vitest/suite-failed.txt", 1, "undetermined", "undetermined"},
		"vitest, a path selecting no file": {
			typescriptID, "vitest/no-test-files.txt", 1, "undetermined", "undetermined"},
		// adds two numbers and adds zero; a skipped and a todo test are not
		// counted.
		"jest, all passing":            {jestID, "jest/all-passing.txt", 0, "2", "0"},
		"jest, some failing":           {jestID, "jest/some-failing.txt", 1, "2", "1"},
		"jest, a filter matching none": {jestID, "jest/no-tests-selected.txt", 0, "0", "0"},
		"jest, a suite that did not load": {
			jestID, "jest/suite-failed.txt", 1, "undetermined", "undetermined"},
		"jest, a path selecting no file": {
			jestID, "jest/no-test-files.txt", 1, "undetermined", "undetermined"},
		"cargo, all passing": {rustID, "cargo/all-passing.txt", 0, "2", "0"},
		// adds_two_numbers, fails_on_purpose, integration_adds and the
		// doc-test; is_ignored is not counted.
		"cargo, some failing":           {rustID, "cargo/some-failing.txt", 101, "4", "1"},
		"cargo, a filter matching none": {rustID, "cargo/no-tests-selected.txt", 0, "0", "0"},
		"cargo, only ignored tests":     {rustID, "cargo/ignored-only.txt", 0, "0", "0"},
		"cargo, a crate that did not build": {
			rustID, "cargo/build-failed.txt", 101, "undetermined", "undetermined"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := loadShipped(t, tc.profile)
			output, err := os.ReadFile(filepath.Join("testdata", tc.file))
			require.NoError(t, err)

			executed, failed := counted(t, &p, string(output), tc.exit)
			assert.Equal(t, tc.executed, executed, "executed count")
			assert.Equal(t, tc.failed, failed, "failed count")
		})
	}

	// The Jest suite that did not load rests on its JSON reading zero, so
	// the premise is asserted rather than assumed.
	broken, err := os.ReadFile("testdata/jest/suite-failed.txt")
	require.NoError(t, err)
	assert.Contains(t, string(broken), `"numFailedTests":0,"numPassedTestSuites":0,"numPassedTests":0`)
}

// §2.4.2 over the root files of real repositories, measured 2026-09-23 on
// tarfin's: a Vue app with only a Vite config, a React Native app configuring
// Jest, an Astro site with only a tsconfig, a Laravel app carrying a Vite
// config, and a Go module with a JavaScript toolchain beside it. A repository
// with a package.json and nothing else selects no profile, and one whose only
// markers are a Jest config and a tsconfig is a tie cr names.
func TestJavaScriptRepositoriesSelectByTheirToolsNotByPackageJSON(t *testing.T) {
	profiles := shippedProfilesDir(t)
	cases := map[string]struct {
		files    []string
		selected string
		tied     []string
	}{
		"a Vue app on Vite":           {[]string{"package.json", "vite.config.js"}, typescriptID, nil},
		"a React Native app on Jest":  {[]string{"package.json", "tsconfig.json", "jest.config.js", "jest.setup.js", "babel.config.js"}, jestID, nil},
		"an Astro site":               {[]string{"package.json", "tsconfig.json"}, typescriptID, nil},
		"a Laravel app with Vite":     {[]string{"artisan", "composer.json", "phpunit.xml", "tests/Pest.php", "package.json", "vite.config.js"}, laravelPestID, nil},
		"a Go module with a frontend": {[]string{"go.mod", "go.sum", "package.json", "tsconfig.json"}, goID, nil},
		"a package.json alone":        {[]string{"package.json"}, "", nil},
		// match.unless: a TypeScript project that configures Jest is a
		// Jest project, not a tie (QA, 2026-09-23).
		"a Jest config and a tsconfig":           {[]string{"jest.config.ts", "tsconfig.json"}, jestID, nil},
		"React on Vite with Jest and a tsconfig": {[]string{"package.json", "tsconfig.json", "vite.config.ts", "jest.config.js"}, jestID, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			for _, marker := range tc.files {
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(repo, marker)), 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(repo, marker), nil, 0o600))
			}

			selection, err := Select(profiles, repo, "")
			require.NoError(t, err)
			assert.Equal(t, tc.selected != "", selection.Selected)
			if tc.selected != "" {
				assert.Equal(t, tc.selected, selection.Profile.ID)
			}
			want := tc.tied
			if want == nil {
				want = []string{}
			}
			assert.Equal(t, want, selection.Tied)
		})
	}
}
