package cli

import (
	"fmt"
	"slices"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// refuseForeignAnchors rejects a record whose anchor does not lie inside the
// unit it names.
//
// §6.1.3 rejects a `unit` the round does not hold, which proves the unit exists
// and never that it is the record's own. Round 13's agent-chosen-grading-boundary
// is what that leaves open: `unit` is the boundary §6.2's `cited` row measures
// "outside the record's own unit" against, and the agent writes it, so a record
// anchored in u1 could name u2 and cite its own hunk as outside evidence. Binding
// the anchor to the named unit is what makes §6.2.1's "no field the agent writes
// can move a location across the boundary" true.
//
// Containment is §6.2.1's, in head coordinates with no side compared, asked of
// unit.Unit.Contains. A RIGHT anchor's lines are head lines already. A LEFT
// anchor's are merge-base lines, which no unit range is recorded in, so they are
// carried into head coordinates through the hunk that removed them — its
// head-side range, which is where §6.2.1 places a hunk's changed lines — and the
// round's diff is read only when some record carries such an anchor.
//
// A LEFT anchor on lines no hunk removed is refused first, with its own reason.
//
// `cr merge` and `cr record` both run it, over the units of the same round, so a
// file `cr merge` wrote never meets at `cr record` an anchor refusal the merge
// could have made: a range spanning two hunks of its unit, a range outside it,
// and a LEFT anchor on a line the diff did not remove are refused at whichever
// command reads the record first, in the same words.
func refuseForeignAnchors(
	owner, repo string, pr int, round *state.Meta, file string, body []byte,
	formed []roundUnit, records []*finding.Finding,
) error {
	hunks, err := leftAnchorHunks(owner, repo, pr, round.Head, records)
	if err != nil {
		return err
	}
	at := state.RecordLines(body)
	for i, record := range records {
		if err := refuseUnremovedLeft(file, at[i], record, hunks); err != nil {
			return err
		}
		if !anchorInsideUnit(&record.Anchor, unitOf(formed, record.Unit), hunks) {
			return &finding.RejectedRecordError{
				File: file, Line: at[i], Field: "anchor",
				Problem: fmt.Sprintf(
					"of record %s is %s %s:%d-%d, which does not lie inside unit %q, the unit this record names; "+
						"§6.2.1 measures a record's own unit by its anchor, so name the unit whose hunk holds it",
					record.ID, record.Anchor.Side, record.Anchor.Path, record.Anchor.StartLine, record.Anchor.Line,
					record.Unit,
				),
			}
		}
	}
	return nil
}

// leftAnchorHunks reads the round's hunks when some record carries a LEFT
// anchor, and nothing otherwise: a RIGHT anchor needs no diff to be measured.
func leftAnchorHunks(owner, repo string, pr int, head string, records []*finding.Finding) ([]git.Hunk, error) {
	for _, record := range records {
		if record.Anchor.Side == git.Left {
			return roundHunks(owner, repo, pr, head)
		}
	}
	return nil, nil
}

// refuseUnremovedLeft is §9.2.1's refusal for one record on line of file.
//
// §9.2.1 has LEFT anchor a removed line and nothing else. A context line of a
// hunk, or a line outside every hunk, is a line GitHub shows on the RIGHT if at
// all, and a review comment sent to it on the LEFT is refused by the
// review-creation call, which loses the whole round's review over one position.
func refuseUnremovedLeft(file string, line int, record *finding.Finding, hunks []git.Hunk) error {
	anchor := &record.Anchor
	if anchor.Side != git.Left {
		return nil
	}
	if _, _, found := removedHeadRange(anchor, hunks); found {
		return nil
	}
	return &finding.RejectedRecordError{
		File: file, Line: line, Field: "anchor",
		Problem: fmt.Sprintf(
			"of record %s is %s %s:%d-%d, and the round's diff does not remove every one of those merge-base lines; "+
				"§9.2.1 has a LEFT anchor name removed lines only, so anchor a line the change kept or added on the RIGHT, at its head line",
			record.ID, anchor.Side, anchor.Path, anchor.StartLine, anchor.Line,
		),
	}
}

// anchorInsideUnit reports whether every line of the anchor lies inside own, in
// head coordinates.
func anchorInsideUnit(anchor *finding.Anchor, own finding.Containment, hunks []git.Hunk) bool {
	if own == nil {
		return false
	}
	start, end := anchor.StartLine, anchor.Line
	if anchor.Side == git.Left {
		var found bool
		start, end, found = removedHeadRange(anchor, hunks)
		if !found {
			return false
		}
	}
	// Both ends first, so the walk below is bounded by the unit's own
	// ranges rather than by whatever line number the agent wrote.
	if !own.Contains(anchor.Path, start) || !own.Contains(anchor.Path, end) {
		return false
	}
	for line := start + 1; line < end; line++ {
		if !own.Contains(anchor.Path, line) {
			return false
		}
	}
	return true
}

// removedHeadRange is the head-side range of the hunk that removed every
// merge-base line of a LEFT anchor, and false when no one hunk of the round's
// diff removed them all.
//
// A hunk's merge-base range, BaseStart for BaseLines, holds its context lines as
// well as its removed ones, so it is not asked: a context line was removed by
// nobody. Removed is ascending and holds no number twice, so the anchor's lines
// are all in it exactly when its first line is and its last line sits as far
// past the first in Removed as in the file. A last line Removed lacks indexes
// at -1, which lies before the first, and finding.ValidateAnchor has already
// refused an anchor whose line lies before its start_line.
func removedHeadRange(anchor *finding.Anchor, hunks []git.Hunk) (start, end int, found bool) {
	for i := range hunks {
		hunk := &hunks[i]
		if hunk.Path != anchor.Path {
			continue
		}
		first := slices.Index(hunk.Removed, anchor.StartLine)
		last := slices.Index(hunk.Removed, anchor.Line)
		if first >= 0 && last-first == anchor.Line-anchor.StartLine {
			start, end = hunk.HeadRange()
			return start, end, true
		}
	}
	return 0, 0, false
}
