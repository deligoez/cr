package testadequacy

import (
	"testing"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/unit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// patch is one round's diff: a source file, a test file changed in two places,
// a test file the pull request adds, and a test file it deletes. It is parsed by
// the real parser rather than hand-built into Hunks, so the paths under test are
// the ones git's own headers yield — including the deleted file's, which has no
// head-side name at all.
const patch = `diff --git a/app/Models/Order.php b/app/Models/Order.php
--- a/app/Models/Order.php
+++ b/app/Models/Order.php
@@ -10,3 +10,4 @@
 class Order
 {
+    public $total;
 }
diff --git a/tests/Feature/OrderTest.php b/tests/Feature/OrderTest.php
--- a/tests/Feature/OrderTest.php
+++ b/tests/Feature/OrderTest.php
@@ -5,2 +5,3 @@
 it('totals', function () {
+    expect(1)->toBe(1);
 });
@@ -20,2 +21,3 @@
 it('discounts', function () {
+    expect(2)->toBe(2);
 });
diff --git a/tests/Unit/PricingTest.php b/tests/Unit/PricingTest.php
--- /dev/null
+++ b/tests/Unit/PricingTest.php
@@ -0,0 +1,2 @@
+<?php
+it('prices', fn () => expect(1)->toBe(1));
diff --git a/tests/Feature/LegacyTest.php b/tests/Feature/LegacyTest.php
--- a/tests/Feature/LegacyTest.php
+++ /dev/null
@@ -1,2 +0,0 @@
-<?php
-it('legacy', fn () => expect(true)->toBeTrue());
`

// changedTests are the test files patch names, in the order it names them.
var changedTests = []string{
	"tests/Feature/OrderTest.php",
	"tests/Unit/PricingTest.php",
	"tests/Feature/LegacyTest.php",
}

// hunks parses patch through the parser §3.4.1 uses.
func hunks(t *testing.T) []git.Hunk {
	t.Helper()
	parsed, err := git.ParseHunks(patch)
	require.NoError(t, err)
	return parsed
}

// laravelPest is the shipped profile, whose `tests.globs` and `symbols.lang`
// are the reviewed bytes rather than a literal written to suit the assertion.
func laravelPest(t *testing.T) profile.Profile {
	t.Helper()
	p, err := profile.Parse("laravel-pest.json", []byte(profile.Builtins()["laravel-pest"]))
	require.NoError(t, err)
	return p
}

// index is a symbol index that answers for the files it holds and reports false
// for the rest, which is what §4.4.1's second half asks of one.
type index map[string][]string

func (i index) Referenced(path string) ([]string, bool) {
	symbols, indexed := i[path]
	return symbols, indexed
}

// §4.4.1 attaches the test files the pull request changed or added, and the
// assertion is the exact set: the source file is out, the test file changed in
// two hunks appears once, and the diff's own order is kept so the same pull
// request always attaches the same list.
//
// The deleted test file is in, and that is the deliberate reading. A deletion is
// neither a new file nor an edited one, but it is the most coverage-relevant
// thing a diff can do to a suite: withholding it would leave the agent
// classifying a unit as covered by a test the same pull request removed, which
// is a wrong assertion reaching the author through cr's own silence.
func TestTheAttachmentIsExactlyTheRoundsChangedTestFiles(t *testing.T) {
	p := laravelPest(t)

	attached := Attach(&p, index{}, hunks(t))

	assert.Equal(t, changedTests, attached.Paths)
}

