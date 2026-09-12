package suggestion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
)

// twoHunkDiff changes one file in two places and empties a third line of
// another, so one fixture carries every shape §8.2 decides between: a range
// inside a hunk, a range whose ends sit in two of them, a line in no hunk at
// all, and a hunk that adds no line and is therefore numbered on LEFT.
//
// The first hunk covers head lines 10-12 and the second 40-41. The deletion
// hunk of the second file covers head line 20 alone, as an insertion point
// rather than as a line that exists to be replaced.
const twoHunkDiff = `--- a/app/Models/Order.php
+++ b/app/Models/Order.php
@@ -10,3 +10,3 @@
 $unchanged = 1;
-$removed = 2;
+$added = 2;
 $tail = 3;
@@ -40,1 +40,2 @@
 $keep = 1;
+$more = 2;
--- a/app/Models/Invoice.php
+++ b/app/Models/Invoice.php
@@ -20,2 +20,1 @@
 $kept = 1;
-$gone = 2;
`

// hunksOf parses the fixture the way a run does, so every case is decided
// against the hunks git.ParseHunks produces rather than against a hand-built
// struct whose head range could be anything.
func hunksOf(t *testing.T) []git.Hunk {
	t.Helper()
	hunks, err := git.ParseHunks(twoHunkDiff)
	require.NoError(t, err)
	return hunks
}

// suggesting is a record carrying a suggestion anchored on the given range.
func suggesting(anchor *finding.Anchor) *finding.Finding {
	return &finding.Finding{
		ID: "f1", Kind: finding.KindFinding, Role: "correctness", Class: "dropped-error",
		Severity: finding.SeverityMedium, Unit: "u1", Anchor: *anchor,
		Summary:    "The added line ignores the error.",
		Evidence:   "The call's second result is assigned to _.",
		Suggestion: "$added = 3;",
	}
}

// right is an anchor on the RIGHT side of one file, which is the only side
// §8.2.1 admits.
func right(path string, start, end int) finding.Anchor {
	return finding.Anchor{Path: path, Side: git.Right, StartLine: start, Line: end}
}

// §8.2.1 and §8.2.2: a contiguous RIGHT-side range inside one hunk is admitted,
// and the hunk's own first and last lines are in it.
//
// Both edges are asserted because a validator that counted either of them out
// would still pass a range sitting strictly inside, and would then drop —
// without a word — the suggestion on every finding anchored on an edge line.
func TestARangeInsideOneHunkIsAdmitted(t *testing.T) {
	hunks := hunksOf(t)
	for _, c := range []struct {
		name   string
		anchor finding.Anchor
	}{
		{name: "one line in the middle of a hunk", anchor: right("app/Models/Order.php", 11, 11)},
		{name: "a range starting on the hunk's first line", anchor: right("app/Models/Order.php", 10, 11)},
		{name: "a range ending on the hunk's last line", anchor: right("app/Models/Order.php", 11, 12)},
		{name: "the whole of a hunk", anchor: right("app/Models/Order.php", 10, 12)},
		{name: "a range in the second hunk", anchor: right("app/Models/Order.php", 40, 41)},
	} {
		t.Run(c.name, func(t *testing.T) {
			assert.NoError(t, Validate(suggesting(&c.anchor), hunks))
			assert.True(t, Placeable(&c.anchor, hunks), "the predicate agrees with the validator")
		})
	}
}

// §8.2.4: a suggestion failing validation is refused, and the refusal names the
// record id.
//
// The cases are the three §8.2 distinguishes plus the two the binding of the
// range to `anchor.start_line`..`anchor.line` makes reachable. A range spanning
// two hunks is the one §8.2.2 adds to §8.2.1: every line of it exists on the
// RIGHT side, and the lines between the two hunks were never in the diff, so
// the hunk a range starts in is what bounds how many lines it may replace.
func TestARangeOutsideOneHunkIsRefusedNamingTheRecord(t *testing.T) {
	hunks := hunksOf(t)
	for _, c := range []struct {
		name   string
		anchor finding.Anchor
		cites  string
	}{
		{name: "§8.2.1: a line the diff does not carry",
			anchor: right("app/Models/Order.php", 400, 400), cites: "§8.2.1"},
		{name: "§8.2.1: a file the diff does not touch",
			anchor: right("app/Services/Ledger.php", 11, 11), cites: "§8.2.1"},
		{name: "§8.2.1: a range on the LEFT side",
			anchor: finding.Anchor{
				Path: "app/Models/Order.php", Side: git.Left, StartLine: 11, Line: 11,
			}, cites: "§8.2.1"},
		{name: "§8.2.1: a hunk that adds no line",
			anchor: right("app/Models/Invoice.php", 20, 20), cites: "§8.2.1"},
		{name: "§8.2.2: a range spanning two hunks",
			anchor: right("app/Models/Order.php", 11, 41), cites: "§8.2.2"},
		{name: "§8.2.1: a range that runs backwards",
			anchor: right("app/Models/Order.php", 12, 10), cites: "§8.2.1"},
		{name: "§8.2.1: a range starting above the first line of the file",
			anchor: right("app/Models/Order.php", 0, 11), cites: "§8.2.1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(suggesting(&c.anchor), hunks)

			var refused *RangeError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, "f1", refused.Record, "§8.2.4: the refusal names the record id")
			assert.Contains(t, err.Error(), "f1")
			assert.Contains(t, err.Error(), c.cites, "the refusal says which clause refused it")
			assert.False(t, Placeable(&c.anchor, hunks), "the predicate agrees with the validator")
		})
	}
}

// A record carrying no suggestion passes, however §8.2 would read its anchor.
//
// §6.1's table makes `suggestion` optional and most records carry none, and
// §8.2 is about where a replacement lands. Refusing a record for an anchor that
// could not hold a suggestion it does not have would block the round over a
// finding the author should simply read.
func TestARecordWithoutASuggestionIsNotHeldToSection82(t *testing.T) {
	anchor := right("app/Models/Order.php", 400, 400)
	record := suggesting(&anchor)
	record.Suggestion = ""

	assert.NoError(t, Validate(record, hunksOf(t)))
}

// The empty side is refused beside LEFT. §9.2 gives every anchor one of two
// sides, so a record carrying neither was not written by cr — and a validator
// testing for LEFT rather than for RIGHT would admit it, replacing lines in a
// tree nothing resolved.
func TestAnAnchorWithNoSideIsRefused(t *testing.T) {
	anchor := finding.Anchor{Path: "app/Models/Order.php", StartLine: 11, Line: 11}

	assert.Error(t, Validate(suggesting(&anchor), hunksOf(t)))
}
