package profile

import (
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/run"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// laravelPestFile is the path Parse is given for the shipped profile. §2.4 ties
// the id to the file stem, so the shipped file is loaded under the name cr
// writes it as rather than under a name invented by the test.
const laravelPestFile = laravelPestID + ".json"

// loadLaravelPest parses the shipped profile, which must be usable by the same
// loader that reads a user's own file: cr ships no profile it would refuse.
func loadLaravelPest(t *testing.T) Profile {
	t.Helper()
	p, err := Parse(laravelPestFile, []byte(laravelPest))
	require.NoError(t, err)
	return p
}

// §2.4.5 requires v0.1 to ship laravel-pest, and a profile that ships with a
// field missing disables the machinery that reads it rather than failing
// loudly: no tests.cmd is a disabled test axis, no tests.filter_flag is an
// unfilterable probe, no symbols.lang is a reinvention search with no language.
// Each field is therefore asserted at the value it must carry for a Laravel
// repository, not merely as non-empty.
func TestTheShippedLaravelPestProfileFillsEveryFieldItNeeds(t *testing.T) {
	p := loadLaravelPest(t)

	assert.Equal(t, laravelPestID, p.ID)
	// artisan is Laravel and tests/Pest.php is Pest; the other two are
	// present in the same repository and raise the count §2.4.2 compares.
	assert.Subset(t, p.Match.Files, []string{"artisan", "tests/Pest.php"})
	assert.Subset(t, p.Match.Globs, []string{"app/**/*.php", "tests/**/*.php"})

	// Every axis of §1.5 is answered, so no axis falls to an absent key.
	for _, id := range axis.IDs() {
		enabled, stated := p.Axes[id]
		assert.True(t, stated, "axes must state %s", id)
		assert.True(t, enabled, "axes must enable %s", id)
	}

	// A git worktree carries tracked files only, so the untracked files a
	// Laravel suite reads are copied, and composer reconciles the copied
	// vendor with the lock file at the head under review. `.env.testing` is
	// among them because Laravel boots its tests under APP_ENV=testing and,
	// when that file is absent, falls back to `.env` — the developer's own
	// application database (spec/field-feedback.md, 2.1).
	assert.Equal(t, []string{".env", ".env.testing", "vendor"}, p.Sandbox.Copy)
	require.Len(t, p.Sandbox.Setup, 1)
	assert.Contains(t, p.Sandbox.Setup[0], "composer install")

	assert.Equal(t, []string{"./vendor/bin/pest", "--colors=never"}, p.Tests.Cmd)
	assert.Equal(t, []string{"tests/**/*Test.php"}, p.Tests.Globs)
	assert.Equal(t, "--filter", p.Tests.FilterFlag)
	assert.NotEmpty(t, p.Tests.CountPattern)
	assert.NotEmpty(t, p.Tests.FailedPattern)
	assert.Equal(t, "php", p.Symbols.Lang)
}

// §2.4.5: every shipped profile carries an empty sandbox.require.
//
// Empty rather than absent is the whole point, and the two are the same fact
// here: §2.4 normalises an absent list to an empty one, so neither shipped file
// states the field and both resolve carrying none. What §5.1.8 then refuses is
// nothing, which is the only honest default — cr ships no opinion about which
// file a stranger's suite cannot run without, and an entry guessed at here
// would refuse every run in a repository that never needed it.
//
// A profile that resolved this field as nil rather than as an empty list would
// pass §5.1.8 the same way and serialise as null, which §12.3 refuses, so the
// assertion is on the value and not on its length.
func TestNoShippedProfileRequiresASandboxPath(t *testing.T) {
	for id, content := range Builtins() {
		t.Run(id, func(t *testing.T) {
			p, err := Parse(id+fileExt, []byte(content))
			require.NoError(t, err)
			assert.Equal(t, []string{}, p.Sandbox.Require,
				"§2.4.5: the shipped profiles require no sandbox path")
		})
	}
	assert.Len(t, Builtins(), 6, "§2.4.5: v0.9 ships laravel-pest, go, typescript, jest, rust and generic")
}

// §5.4.2 places a gap probe's test at tests.probe_path_template and then runs
// the suite, so a template the runner does not collect turns every gap probe
// into a run of the untouched suite — a green result that proves nothing and
// looks exactly like proof. Two conditions decide it, and §2.4 names both: the
// path must satisfy the profile's own tests.globs, and the runner's discovery
// must find the file there.
//
// The default template would satisfy neither. It roots at the first glob's
// literal prefix, tests, and a file directly under tests/ belongs to no
// testsuite in the phpunit.xml Laravel ships, whose suites are tests/Unit and
// tests/Feature. Hence the explicit template. Pest 4.7.8 was observed
// collecting a file written to exactly this path and skipping one written a
// directory higher.
func TestTheLaravelPestProbePathSatisfiesItsTestGlobs(t *testing.T) {
	p := loadLaravelPest(t)

	placed := p.ProbePath("p7", "app/Services")

	assert.Equal(t, "tests/Feature/cr_probe_p7Test.php", placed)
	// The glob's two literal anchors, read off the profile rather than
	// restated: everything before its first wildcard and everything after
	// its last.
	root := probeRoot(p.Tests.Globs[0])
	ext, unresolvable := probeExt(p.Tests.Globs)
	require.Empty(t, unresolvable)
	assert.True(t, strings.HasPrefix(placed, root+"/"), "%s must sit under %s", placed, root)
	assert.True(t, strings.HasSuffix(placed, ext), "%s must end with %s", placed, ext)
	// The directory is a registered testsuite, which is the half of
	// discovery the globs cannot express.
	assert.Equal(t, "tests/Feature", path.Dir(placed))
	// §5.1.6 recognises a leftover artefact by replacing the probe id,
	// which only works while the rest of the path stays fixed.
	assert.Equal(t, "tests/Feature/cr_probe_*Test.php", p.LeftoverGlob())
}

// counted runs §5.2.1's extraction over output with the shipped patterns, and
// renders one of its two counts so an undetermined one reads as the answer it
// is rather than as a zero.
//
// The arithmetic is run.Counter's rather than this file's. It used to be
// written out here, because summing the patterns was §5.2.1's implementation
// and did not exist yet; a copy kept now would be a second implementation of
// the rule that can agree with the spec while the one cr ships does not.
func counted(t *testing.T, p *Profile, output string, exitCode int) (executed, failed string) {
	t.Helper()
	counter, err := run.NewCounter(p.Tests.CountPattern, p.Tests.FailedPattern, p.CountsOccurrences())
	require.NoError(t, err)
	_, err = counter.Write([]byte(output))
	require.NoError(t, err)
	ran, broke := counter.Counts(exitCode)
	return shown(ran), shown(broke)
}

// shown renders one of §5.2.4's optional counts.
func shown(n *int) string {
	if n == nil {
		return "undetermined"
	}
	return strconv.Itoa(*n)
}

// The recap files under testdata/pest are the real output of Pest 4.7.8 on PHP
// 8.5.9, captured rather than written, so the patterns are judged by the text
// they will actually meet — failure diffs, source excerpts, and the bare line
// numbers a failure list prints included.
//
// Three readings matter. An all-passing run never writes the word failed, so
// the failed count comes from no match at all and must be zero rather than
// undetermined; that is the baseline §5.2.5 has to be able to pass. A mixed run
// counts only the tests that ran: todo and skipped were selected and never
// executed, and counting them would let a run of nothing but skipped tests
// report no failures and hand §5.3.5 the proof a gap finding rests on. And a
// run that selected nothing prints no recap line at all, so both counts are
// undetermined and §5.3.4 stops at rung 5, inconclusive, which supports no
// grade either.
func TestTheLaravelPestPatternsCountCapturedPestOutput(t *testing.T) {
	p := loadLaravelPest(t)

	cases := map[string]struct {
		file     string
		executed string
		failed   string
	}{
		"an all-passing run": {"all-passing.txt", "2", "0"},
		"a failing run":      {"some-failing.txt", "4", "1"},
		// 2 failed and 4 passed ran; 1 todo and 1 skipped did not.
		"every status at once": {"mixed-statuses.txt", "6", "2"},
		"nothing selected":     {"no-tests-found.txt", "undetermined", "undetermined"},
		"only skipped tests":   {"skipped-only.txt", "undetermined", "undetermined"},
		"only todo tests":      {"todo-only.txt", "undetermined", "undetermined"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			output, err := os.ReadFile("testdata/pest/" + tc.file)
			require.NoError(t, err)

			executed, failed := counted(t, &p, string(output), 0)
			assert.Equal(t, tc.executed, executed, "executed count")
			assert.Equal(t, tc.failed, failed, "failed count")
		})
	}

	// The all-passing case rests on the word never appearing, so the
	// premise is asserted rather than assumed: its zero comes from no
	// match at all, not from a line reading zero failures.
	passing, err := os.ReadFile("testdata/pest/all-passing.txt")
	require.NoError(t, err)
	assert.NotContains(t, string(passing), "failed",
		"§5.2.1: Pest prints no status whose count is zero")
}
