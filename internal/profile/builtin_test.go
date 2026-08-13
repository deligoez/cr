package profile

import (
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/axis"
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

	// A git worktree carries tracked files only, so the two a Laravel suite
	// cannot boot without are copied, and composer reconciles the copied
	// vendor with the lock file at the head under review.
	assert.Equal(t, []string{".env", "vendor"}, p.Sandbox.Copy)
	require.Len(t, p.Sandbox.Setup, 1)
	assert.Contains(t, p.Sandbox.Setup[0], "composer install")

	assert.Equal(t, []string{"./vendor/bin/pest", "--colors=never"}, p.Tests.Cmd)
	assert.Equal(t, []string{"tests/**/*Test.php"}, p.Tests.Globs)
	assert.Equal(t, "--filter", p.Tests.FilterFlag)
	assert.NotEmpty(t, p.Tests.CountPattern)
	assert.NotEmpty(t, p.Tests.FailedPattern)
	assert.Equal(t, "php", p.Symbols.Lang)
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

	placed := p.ProbePath("p7")

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

// §5.2.1 sums each pattern over every match and reads an unmatched
// tests.failed_pattern as zero. sumPattern is that arithmetic, kept here
// because running the patterns is §5.2.1's implementation and not this
// package's: what the test proves is that the shipped patterns yield the right
// numbers from output Pest really produced.
func sumPattern(t *testing.T, pattern, output string) (int, bool) {
	t.Helper()
	compiled, err := regexp.Compile(pattern)
	require.NoError(t, err)
	matches := compiled.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0, false
	}
	total := 0
	for _, match := range matches {
		n, err := strconv.Atoi(match[1])
		require.NoError(t, err)
		total += n
	}
	return total, true
}

