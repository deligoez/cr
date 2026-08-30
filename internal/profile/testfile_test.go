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

// A `**` at the end of a glob selects every file beneath it, however deep.
//
// It is the boundary the shipped `tests/**/*Test.php` never reaches, because
// there `**` always has a segment behind it to stop at. A suite whose every file
// under `tests/` is a test writes `tests/**` instead, and then `**` has to
// consume the whole remaining path rather than all but the last segment — an
// off-by-one that mutation testing surfaced and that no glob in the repository
// would have found. It fails the same silent way the depth cases do: files
// recognised as tests everywhere except at the bottom of the tree.
//
// Nothing here asserts what `tests/**` does with the bare directory `tests`. A
// diff header names a file, never a directory, so the question cannot arise from
// anything §4.4.1 reads.
func TestATrailingDoubleStarSelectsEveryFileBeneathIt(t *testing.T) {
	p := Profile{Tests: Tests{Globs: []string{"tests/**"}}}

	assert.True(t, p.IsTestFile("tests/OrderTest.php"))
	assert.True(t, p.IsTestFile("tests/Feature/OrderTest.php"))
	assert.True(t, p.IsTestFile("tests/Feature/Api/Billing/InvoiceTest.php"))

	assert.False(t, p.IsTestFile("app/Models/Order.php"))
}

// §2.4 makes `tests.globs` required whenever `tests.cmd` is present and §4.5.2
// disables the test axis when `tests.cmd` is absent, so a profile that can run
// tests can always name them. generic is the profile that declares neither, and
// the assertion is that it recognises nothing rather than everything: a matcher
// reading an empty glob list as a wildcard would attach every changed file in
// the repository as a test file, and the agent would classify a unit as covered
// by the source it was supposed to be reviewing.
func TestAProfileWithNoTestGlobsRecognisesNoTestFile(t *testing.T) {
	p, err := Parse(genericID+fileExt, []byte(Builtins()[genericID]))
	require.NoError(t, err)
	require.Empty(t, p.Tests.Globs)
	// The half of §4.4 that needs no symbol index still needs this, and
	// generic's silence here is §4.5.2's disabled axis rather than a lens
	// that looked and found nothing.
	require.Empty(t, p.Tests.Cmd)

	assert.False(t, p.IsTestFile("tests/OrderTest.php"))
	assert.False(t, p.IsTestFile("app/Models/Order.php"))
}

// A glob path.Match cannot compile has no matches rather than a panic or a
// silent match on everything. §2.4 states no syntax rule for `tests.globs`, so
// the matcher does not invent a rejection — but the two ways it could go wrong
// both end with cr attaching files it never established are tests, which is the
// assertion worth pinning.
func TestAnUncompilableGlobMatchesNothing(t *testing.T) {
	p := Profile{Tests: Tests{Globs: []string{"tests/[Order"}}}

	assert.False(t, p.IsTestFile("tests/[Order"))
	assert.False(t, p.IsTestFile("tests/OrderTest.php"))
}
