package cli

import (
	"fmt"

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
func refuseForeignAnchors(
	owner, repo string, pr int, round *state.Meta, file string, body []byte,
	formed []roundUnit, records []*finding.Finding,
) error {
	at := state.RecordLines(body)
	var hunks []git.Hunk
	for i, record := range records {
		if record.Anchor.Side == git.Left && hunks == nil {
			read, err := roundHunks(owner, repo, pr, round.Head)
			if err != nil {
				return err
			}
			hunks = read
		}
		if !anchorInsideUnit(&record.Anchor, unitOf(formed, record.Unit), hunks) {
			return &finding.RejectedRecordError{
				File: file, Line: at[i], Field: "anchor",
				Problem: fmt.Sprintf(
					"%s %s:%d-%d does not lie inside unit %q, the unit this record names; "+
						"§6.2.1 measures a record's own unit by its anchor, so name the unit whose hunk holds it",
					record.Anchor.Side, record.Anchor.Path, record.Anchor.StartLine, record.Anchor.Line, record.Unit,
				),
			}
		}
	}
	return nil
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

// removedHeadRange is the head-side range of the hunk whose merge-base lines
// hold a LEFT anchor whole, and false when no hunk of the round's diff removed
// those lines.
func removedHeadRange(anchor *finding.Anchor, hunks []git.Hunk) (start, end int, found bool) {
	for i := range hunks {
		hunk := &hunks[i]
		if hunk.Path != anchor.Path || hunk.BaseLines == 0 {
			continue
		}
		last := hunk.BaseStart + hunk.BaseLines - 1
		if anchor.StartLine >= hunk.BaseStart && anchor.Line <= last {
			start, end = hunk.HeadRange()
			return start, end, true
		}
	}
	return 0, 0, false
}
