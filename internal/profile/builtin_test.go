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

