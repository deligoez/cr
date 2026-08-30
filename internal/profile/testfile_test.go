package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shippedLaravelPest parses the profile cr ships, so the globs under test are
// the reviewed bytes of builtin/laravel-pest.json rather than a literal written
// to suit the assertion.
func shippedLaravelPest(t *testing.T) Profile {
	t.Helper()
	p, err := Parse(laravelPestID+fileExt, []byte(Builtins()[laravelPestID]))
	require.NoError(t, err)
	return p
}

// §4.4.1 attaches "the test files changed or added by the PR", and which files
// those are is `tests.globs` and nothing else. The shipped laravel-pest glob is
// `tests/**/*Test.php`, so the depth cases are the assertion: a suite keeps its
// tests at the root of `tests/` and several directories down, and `**` has to
// stand for both — including for no directory at all.
//
// Reading `**` as path.Match's `*` would match exactly the one-directory case
// and miss the other two. That failure is silent and expensive: §4.4.1 would
// attach a fraction of the suite, the agent would classify a unit against the
// tests it was shown, and neither it nor the author would be told that the rest
// of the suite was never in the room.
func TestTestGlobsSelectTestFilesAtEveryDepth(t *testing.T) {
	p := shippedLaravelPest(t)

	for _, file := range []string{
		"tests/UnitTest.php",
		"tests/Feature/OrderTest.php",
		"tests/Feature/Api/Billing/InvoiceTest.php",
	} {
		assert.True(t, p.IsTestFile(file), "%s is a test file of tests/**/*Test.php", file)
	}

	for _, file := range []string{
		// Under tests/, but not a test file: the suffix is part of
		// the glob and Pest.php is the suite's bootstrap.
		"tests/Pest.php",
		// A test-shaped name outside the test root. app/ is source,
		// and §4.4.1 attaches tests.
		"app/Models/OrderTest.php",
		// The test root's own name is a segment, not a prefix.
		"testsuite/OrderTest.php",
		"",
	} {
		assert.False(t, p.IsTestFile(file), "%q is not a test file of tests/**/*Test.php", file)
	}
}

