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
// An anchor on lines the diff did not change is refused first, with its own
// reason: a LEFT anchor on lines no hunk removed, and a RIGHT anchor naming a
// unit made only of removed lines.
//
// `cr merge` and `cr record` both run it, over the units of the same round, so a
// file `cr merge` wrote never meets at `cr record` an anchor refusal the merge
// could have made: a range spanning two hunks of its unit, a range outside it,
// and an anchor on lines the diff did not change are refused at whichever
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
		if err := refuseUnchangedLines(file, at[i], record, sideOf(formed, record.Unit), hunks); err != nil {
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

// refuseUnchangedLines is §9.2.1's refusal for one record on line of file, whose
// unit is on side.
//
// §9.2.1 has LEFT anchor a removed line and nothing else. A context line of a
// hunk, or a line outside every hunk, is a line GitHub shows on the RIGHT if at
// all, and a review comment sent to it on the LEFT is refused by the
// review-creation call, which loses the whole round's review over one position.
//
// The same sentence has LEFT anchor every record about a deletion, and a unit
// whose side is LEFT is made only of removed lines: every head line it can
// reach, its insertion point included, is one the diff did not change. A RIGHT
// anchor naming such a unit would be graded, and could reach `probed`, as an
// assertion about a line the pull request left alone.
func refuseUnchangedLines(file string, line int, record *finding.Finding, side git.Side, hunks []git.Hunk) error {
	anchor := &record.Anchor
	var problem string
	switch {
	case anchor.Side == git.Right && side == git.Left:
		problem = fmt.Sprintf(
			"of record %s is %s %s:%d-%d, and unit %q only removes lines, so the diff changed no head line of it; "+
				"§9.2.1 anchors a record about a deletion on the LEFT, so anchor the merge-base lines the unit removes",
			record.ID, anchor.Side, anchor.Path, anchor.StartLine, anchor.Line, record.Unit)
	case anchor.Side == git.Left:
		if _, _, found := removedHeadRange(anchor, hunks); found {
			return nil
		}
		problem = fmt.Sprintf(
			"of record %s is %s %s:%d-%d, and the round's diff does not remove every one of those merge-base lines; "+
				"§9.2.1 has a LEFT anchor name removed lines only, so anchor the lines a hunk removes, "+
				"or on the RIGHT a head line of a unit that adds lines",
			record.ID, anchor.Side, anchor.Path, anchor.StartLine, anchor.Line)
	default:
		return nil
	}
	return &finding.RejectedRecordError{File: file, Line: line, Field: "anchor", Problem: problem}
}

// unchangedAnchors is refuseUnchangedLines for §7.2's location row: a draft's
// marker that moves a record's anchor is held to the refusal `cr record` makes
// of the same anchor, over the units of the same round.
//
// The round's units are read the first time a moved anchor is asked about, and
// its diff the first time that anchor is LEFT, so a draft whose markers move
// nothing reads neither, as anchorTrees keeps it. The refusal's file and line
// are left empty: draft.Ingest names the record and the marker's draft line.
func unchangedAnchors(l state.Layout, owner, repo string, pr int, round *state.Meta) func(*finding.Finding) error {
	var formed []roundUnit
	var hunks []git.Hunk
	var unitsRead, hunksRead bool
	return func(record *finding.Finding) error {
		if !unitsRead {
			read, err := roundUnitsOf(l, owner, repo, pr, round.Round)
			if err != nil {
				return err
			}
			formed, unitsRead = read, true
		}
		if record.Anchor.Side == git.Left && !hunksRead {
			read, err := roundHunks(owner, repo, pr, round.Head)
			if err != nil {
				return err
			}
			hunks, hunksRead = read, true
		}
		return refuseUnchangedLines("", 0, record, sideOf(formed, record.Unit), hunks)
	}
}

// sideOf is the side of the round's unit id, and empty when the round holds no
// unit by that id, which anchorInsideUnit then refuses on its own.
func sideOf(formed []roundUnit, id string) git.Side {
	for i := range formed {
		if formed[i].ID == id {
			return formed[i].Side
		}
	}
	return ""
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
