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

// §4.4.1 attaches to *every* unit, and every unit gets the same attachment.
//
// The sameness is the rule, not an implementation shortcut. Narrowing one unit's
// test files to the ones that bear on it is precisely the coverage judgement
// §2.1.3 reserves for the agent, and cr making it one step early — while calling
// the result an attachment — would be the tool forming the opinion P5 forbids,
// in the one place nobody would look for it. So the units decide the count here
// and nothing else, and the assertion is that no unit is shown a different set.
func TestEveryUnitIsAttachedTheSameTestFiles(t *testing.T) {
	p := laravelPest(t)
	clusters := unit.Clusters(hunks(t), &p, nil, 10)
	require.Greater(t, len(clusters), 1, "the patch has to form more than one unit for this to say anything")

	attached := Attach(&p, index{}, hunks(t))
	per := PerUnit(clusters, attached)

	require.Len(t, per, len(clusters))
	for i, one := range per {
		assert.Equal(t, attached, one, "unit %d was shown a different attachment", i)
	}
}

// §4.4.1 wants "the symbols they reference", and that half needs a symbol index.
// §4.4 states no unavailability path the way §4.3.1 does, so the round-12 finding
// missing-unavailability-path routes it through §4.5.4 instead: the half that
// could not run is named with its reason.
//
// Both ways it can be missing are covered, because they are different facts
// about the run and a reader acts on them differently — a profile that declares
// no `symbols.lang` is fixed in the profile, and an index cr did not build is
// not. The third state is a partial one, and it is the reason References answers
// with a bool: the symbols cr did read are still attached, and the files it could
// not read are named rather than absorbed into the same empty list.
//
// The failure this guards against is an empty `Symbols` with nothing said. That
// reads to the agent as "cr looked and the tests reference nothing", which is an
// assertion cr never made, and it is the one input to a coverage classification
// that would be silently missing.
func TestTheSymbolHalfIsMarkedUnavailableRatherThanLeftEmpty(t *testing.T) {
	p := laravelPest(t)
	full := index{
		"tests/Feature/OrderTest.php":  {"Order", "Order::total"},
		"tests/Unit/PricingTest.php":   {"Order::total", "Pricing"},
		"tests/Feature/LegacyTest.php": nil,
	}

	t.Run("an index that answers for every file leaves nothing unavailable", func(t *testing.T) {
		attached := Attach(&p, full, hunks(t))

		assert.Equal(t, []string{"Order", "Order::total", "Pricing"}, attached.Symbols)
		require.NotNil(t, attached.Unavailable)
		assert.Empty(t, attached.Unavailable)
	})

	t.Run("no symbols.lang", func(t *testing.T) {
		none := p
		none.Symbols = profile.Symbols{}

		attached := Attach(&none, full, hunks(t))

		assert.Empty(t, attached.Symbols)
		require.Len(t, attached.Unavailable, 1)
		assert.Equal(t, SymbolLens, attached.Unavailable[0].Lens)
		assert.Contains(t, attached.Unavailable[0].Reason, "symbols.lang")
		// The file half is untouched by the symbol half's absence.
		assert.Equal(t, changedTests, attached.Paths)
	})

	t.Run("no index built", func(t *testing.T) {
		attached := Attach(&p, nil, hunks(t))

		assert.Empty(t, attached.Symbols)
		require.Len(t, attached.Unavailable, 1)
		assert.Equal(t, SymbolLens, attached.Unavailable[0].Lens)
		assert.Contains(t, attached.Unavailable[0].Reason, "php")
		assert.Equal(t, changedTests, attached.Paths)
	})

	t.Run("an index that cannot answer for one file", func(t *testing.T) {
		partial := index{"tests/Feature/OrderTest.php": {"Order"}}

		attached := Attach(&p, partial, hunks(t))

		assert.Equal(t, []string{"Order"}, attached.Symbols)
		require.Len(t, attached.Unavailable, 1)
		assert.Contains(t, attached.Unavailable[0].Reason, "tests/Unit/PricingTest.php")
		assert.Contains(t, attached.Unavailable[0].Reason, "tests/Feature/LegacyTest.php")
		assert.NotContains(t, attached.Unavailable[0].Reason, "tests/Feature/OrderTest.php")
	})
}

// §11.1 exempts every lens of §4.5.4 that did not run from `--quiet`, and the
// exemption belongs to the writer rather than to each call site, so the report
// has to arrive there as a disclosure. Implementing finding.HonestyDisclosure is
// what makes that possible before the writer exists — the same contract
// profile.MissingProfile already satisfies for §4.3.1's half, so §4.5.4 collects
// both through one interface instead of two shapes.
//
// The text is asserted against the fields rather than against a literal, because
// the two must not be able to drift: a half counted as out in the data and left
// out of the printed report would be honest to a caller reading JSON and silent
// to the human reading a terminal.
func TestAnUnavailableHalfIsAnHonestyDisclosure(t *testing.T) {
	var _ finding.HonestyDisclosure = Unavailable{}

	p := laravelPest(t)
	attached := Attach(&p, nil, hunks(t))
	require.Len(t, attached.Unavailable, 1)

	disclosure := attached.Unavailable[0].Disclosure()
	assert.Contains(t, disclosure, attached.Unavailable[0].Lens)
	assert.Contains(t, disclosure, attached.Unavailable[0].Reason)
	// §4.5 spends `disabled` and `unavailable` on two different states, and
	// this half is the second: the test axis is on, and one of its inputs
	// could not be produced.
	assert.Contains(t, disclosure, "unavailable")
}
