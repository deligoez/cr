package profile

import (
	"os"
	"path/filepath"
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

// §2.4.5: each new profile carries what the probe machinery needs to work at
// all, and each field is asserted at its value, for the reason go's are.
func TestTheShippedTypescriptAndRustProfilesFillEveryFieldTheyNeed(t *testing.T) {
	ts := loadShipped(t, typescriptID)
	assert.Equal(t, []string{"package.json", "tsconfig.json", "vite.config.ts", "vite.config.js"}, ts.Match.Files)
	assert.Equal(t, []string{"npx", "--no", "vitest", "run", "--reporter=verbose"}, ts.Tests.Cmd)
	assert.Equal(t, "-t", ts.Tests.FilterFlag)
	assert.Equal(t, "typescript", ts.Symbols.Lang)

	rs := loadShipped(t, rustID)
	assert.Equal(t, []string{"Cargo.toml", "Cargo.lock"}, rs.Match.Files)
	assert.Equal(t, []string{"cargo", "test", "--no-fail-fast"}, rs.Tests.Cmd)
	assert.Equal(t, "--", rs.Tests.FilterFlag)
	assert.Equal(t, "rust", rs.Symbols.Lang)

	for _, p := range []Profile{ts, rs} {
		assert.True(t, p.CountsOccurrences(), "%s: neither recap has one number to sum", p.ID)
		assert.Equal(t, []string{}, p.Sandbox.Require, p.ID)
		for _, id := range axis.IDs() {
			assert.True(t, p.Axes[id], "%s: axes must enable %s", p.ID, id)
		}
	}
}

// §5.2.1's argv. Vitest takes a path as a file filter; cargo takes a filter
// after `--` and has no path argument, so `--path` is refused rather than
// dropped.
func TestTheTypescriptAndRustArgvNarrowTheWayTheirRunnersDo(t *testing.T) {
	ts := loadShipped(t, typescriptID)
	scoped, err := ts.TestArgv(typescriptID+fileExt, "adds", []string{"src/math.test.ts"})
	require.NoError(t, err)
	assert.Equal(t, []string{"npx", "--no", "vitest", "run", "--reporter=verbose", "-t", "adds", "src/math.test.ts"}, scoped)

	rs := loadShipped(t, rustID)
	filtered, err := rs.TestArgv(rustID+fileExt, "adds", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"cargo", "test", "--no-fail-fast", "--", "adds"}, filtered)

	_, err = rs.TestArgv(rustID+fileExt, "", []string{"src"})
	var unavailable *UnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, testPathsArgField, unavailable.Field)
}

// §2.4's table: each shipped profile's probe path matches its own test globs,
// so a placed gap probe is a file the runner collects.
func TestTheTypescriptAndRustProbePathsAreTestFiles(t *testing.T) {
	ts := loadShipped(t, typescriptID)
	placed := ts.ProbePath("p3", "src/cart")
	assert.Equal(t, "src/cart/cr_probe_p3.test.ts", placed)
	assert.True(t, glob.MatchAny(ts.Tests.Globs, placed))

	rs := loadShipped(t, rustID)
	placed = rs.ProbePath("p3", "src/cart")
	assert.Equal(t, "tests/cr_probe_p3.rs", placed)
	assert.True(t, glob.MatchAny(rs.Tests.Globs, placed))
}

// The files under testdata/vitest are the real output of Vitest 5.0.1 and
// under testdata/cargo of cargo 1.98.1, both on darwin/arm64, captured
// 2026-09-23 with the exit status each run returned.
//
// The readings that matter: a skipped or ignored test is not an executed one,
// so a run of nothing else reads zero; a filter matching nothing exits 0 and
// is zero; and a run that loaded or built no test — a suite whose import
// failed, a path selecting no file, a crate that did not compile — prints no
// test line and exits non-zero, so it is undetermined rather than a run of
// nothing.
func TestTheTypescriptAndRustPatternsCountCapturedOutput(t *testing.T) {
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
}

// §2.4.2's tie, which go.sum is there to avoid: a Go module that also carries
// a package.json selects go, and one that carries a tsconfig.json beside it is
// a tie cr names rather than breaks.
func TestAGoModuleWithAPackageJSONStillSelectsGo(t *testing.T) {
	profiles := shippedProfilesDir(t)
	repo := t.TempDir()
	for _, marker := range []string{"go.mod", "go.sum", "package.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(repo, marker), nil, 0o600))
	}

	selection, err := Select(profiles, repo, "")
	require.NoError(t, err)
	require.True(t, selection.Selected)
	assert.Equal(t, goID, selection.Profile.ID)

	require.NoError(t, os.WriteFile(filepath.Join(repo, "tsconfig.json"), nil, 0o600))
	selection, err = Select(profiles, repo, "")
	require.NoError(t, err)
	assert.Equal(t, []string{goID, typescriptID}, selection.Tied)
}
