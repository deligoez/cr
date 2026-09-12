// Package suggestion holds spec/0.1.0.md §8.2's validation of where a
// suggestion may land.
//
// The range a suggestion replaces is the record's own anchored range,
// `anchor.start_line`..`anchor.line`. §8.2 never says so in as many words —
// round 12's finding suggestion-range-unbound is that §8.2.1's "contiguous line
// range", §8.2.2's "one hunk" and §8.2.3's "first replaced line" each rest on a
// binding nothing states — and the anchor is the only range a record carries.
// It is also the range GitHub uses: a suggestion block replaces the lines its
// comment is attached to, so a one-line fix on a record anchored across four
// lines replaces all four. Fixing the binding here rather than at each caller
// is what keeps the bound a property of the record and not of whoever asks.
//
// Two callers ask, and they ask at different times. §2.6.2.2 validates a
// rule-generated suggestion before drafting and drops the one that fails, and
// §8.4.1 pre-validates every comment position before the review call, because
// that call is atomic and a single bad position loses the whole round. One
// reading serves both: a suggestion the draft accepted and the post refused
// would be a suggestion the author was shown and never offered.
package suggestion

import (
	"fmt"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
)

// RangeError refuses a suggestion §8.2 does not let reach the author.
//
// It names the record, which §8.2.4 requires of the refusal: the reviewer's fix
// is to edit or drop one block, and the record id is how they find it. The cli
// layer maps it onto exit code 1, the code §8.2.4 fixes.
type RangeError struct {
	// Record is the id of the record whose suggestion is refused.
	Record string
	// Problem completes the sentence naming what is wrong with it.
	Problem string
}

func (e *RangeError) Error() string {
	return fmt.Sprintf("the suggestion on record %s %s", e.Record, e.Problem)
}

// Validate holds one record's suggestion to §8.2.1 and §8.2.2, returning a
// RangeError naming the record when the range it would replace is one the diff
// cannot carry.
//
// A record with no suggestion passes. §6.1's table makes `suggestion` optional
// and most records carry none, and §8.2 is about where a replacement lands, not
// about whether a comment is anchored — an anchor §8.2 would refuse still
// carries a finding the author should read, so refusing the record here would
// silence the finding over a suggestion it does not have.
func Validate(record *finding.Finding, hunks []git.Hunk) error {
	if record.Suggestion == "" {
		return nil
	}
	problem := problemWith(&record.Anchor, hunks)
	if problem == "" {
		return nil
	}
	return &RangeError{Record: record.ID, Problem: problem}
}

// Placeable reports whether §8.2 admits a suggestion replacing the anchored
// range. It is Validate's predicate, for the caller that has no record to name
// yet: §2.6.2.2 asks before the generated text reaches the record, so that a
// suggestion that fails is dropped rather than written and then refused.
func Placeable(anchor *finding.Anchor, hunks []git.Hunk) bool {
	return problemWith(anchor, hunks) == ""
}

// problemWith returns the sentence completing a RangeError, or the empty string
// when §8.2 admits the range.
//
// The RIGHT-side test is asked of the anchor and of the hunk both. §9.2 numbers
// a LEFT anchor in the merge base, where a replacement would rewrite a line the
// change already removed; and a hunk that adds no line is numbered on LEFT with
// a head range that is an insertion point rather than a line, so a suggestion
// landing in it would replace nothing that exists at the head.
//
// Containment is asked of the whole range against one hunk, which is the bound
// §8.2.2 puts on §8.2.1: a range whose two ends sit in two hunks is contiguous
// in the file and not in the diff, and the lines between them are lines GitHub
// was never shown. So the hunk the range starts in is what limits how many
// lines a suggestion may replace, and a range is refused for spanning two hunks
// even though every line of it exists on the RIGHT side.
func problemWith(anchor *finding.Anchor, hunks []git.Hunk) string {
	if anchor.Side != git.Right {
		return fmt.Sprintf(
			"is anchored on side %q, and §8.2.1 admits a range on the RIGHT side of the diff alone",
			anchor.Side)
	}
	if anchor.StartLine < 1 || anchor.Line < anchor.StartLine {
		return fmt.Sprintf(
			"replaces lines %d-%d of %s, which is not the contiguous range §8.2.1 requires",
			anchor.StartLine, anchor.Line, anchor.Path)
	}

	var holdsStart, holdsEnd bool
	for at := range hunks {
		hunk := &hunks[at]
		if hunk.Path != anchor.Path || hunk.Side != git.Right {
			continue
		}
		start, end := hunk.HeadRange()
		if anchor.StartLine >= start && anchor.Line <= end {
			return ""
		}
		holdsStart = holdsStart || (anchor.StartLine >= start && anchor.StartLine <= end)
		holdsEnd = holdsEnd || (anchor.Line >= start && anchor.Line <= end)
	}
	if holdsStart && holdsEnd {
		return fmt.Sprintf(
			"replaces lines %d-%d of %s, which no one hunk of the diff covers, and §8.2.2 keeps a range within one hunk",
			anchor.StartLine, anchor.Line, anchor.Path)
	}
	return fmt.Sprintf(
		"replaces lines %d-%d of %s, which the diff does not carry on the RIGHT side, and §8.2.1 requires a range that exists in it",
		anchor.StartLine, anchor.Line, anchor.Path)
}
