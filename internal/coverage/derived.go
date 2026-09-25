package coverage

import (
	"fmt"
	"slices"
)

// keyFields is §4.5.6's key, by the JSON names coverage.ndjson holds a cell's
// under: a cell sits at one `(unit, role)`, and that pair is what a recording
// replaces.
var keyFields = []string{"unit", "role"}

// KeyFields returns §4.5.6's key. The result is a copy.
func KeyFields() []string {
	return slices.Clone(keyFields)
}

// The two cells cr records itself, and the only two. §4.5.6 forbids cr to
// invent a cell for a unit no role reported on, and each of these rests on a
// fact other than cr's judgement of the code: KindCell on the profile's
// declaration that a role does not read a kind of unit (§4.6.7), TwinCell on a
// role's cell at a unit whose hunks the twin repeats line for line (§4.6.8).
// Neither says anything about code no role looked at.

// KindCell is §4.6.7's cell: the `na` cr records at a role the unit's kind does
// not list, with a reason naming the kind.
func KindCell(unitID, unitHash, roleID, kind string) *Cell {
	return &Cell{
		Unit: unitID, Role: roleID, Result: ResultNA, UnitHash: unitHash,
		Reason: fmt.Sprintf("unit kind %s: the profile's units.kinds entry for %s does not list role %s, "+
			"so cr emitted no prompt and records this cell itself (§4.6.7)", kind, kind, roleID),
	}
}

// Twin is one unit of the round that is §4.6.8's twin of an earlier one.
type Twin struct {
	// Unit and Hash are the twin's id and unit hash.
	Unit string
	Hash string
	// Of is the earlier unit it repeats.
	Of string
}

// TwinCell is §4.6.8's cell: the result of the earlier unit's cell, recorded at
// the twin's seat with a reason naming the earlier unit.
//
// The result is carried and nothing else. A `coverage` classification rests on
// the test files the earlier unit's cell named, and a `note_id` on the note that
// explained the earlier unit; neither is a statement about the twin.
func TwinCell(earlier *Cell, twin Twin) *Cell {
	return &Cell{
		Unit: twin.Unit, Role: earlier.Role, Result: earlier.Result, UnitHash: twin.Hash,
		Reason: fmt.Sprintf("twin of %s: its hunks repeat %s's line for line, so cr records %s's %s "+
			"result here (§4.6.8)", twin.Of, twin.Of, twin.Of, earlier.Role),
	}
}

// TwinCells is §4.6.8 over one recording: for every cell at a unit some twin
// repeats, that twin's cell, in the order the cells and then the twins arrive.
// A seat named by recorded itself is left to the cell recorded there.
func TwinCells(recorded []*Cell, twins []Twin) []*Cell {
	named := make(map[Seat]bool, len(recorded))
	for _, cell := range recorded {
		named[Seat{Unit: cell.Unit, Role: cell.Role}] = true
	}
	copies := make([]*Cell, 0)
	for _, cell := range recorded {
		for _, twin := range twins {
			seat := Seat{Unit: twin.Unit, Role: cell.Role}
			if twin.Of != cell.Unit || named[seat] {
				continue
			}
			named[seat] = true
			copies = append(copies, TwinCell(cell, twin))
		}
	}
	return copies
}
