package coverage

import "github.com/deligoez/cr/internal/unit"

// Rows is one round's coverage state, counted the way §10.1.1 counts it: units
// total, units with a complete row of cells, units with gaps, and units flagged
// `oversized` per §3.4.5 — and the number of active roles a row is complete
// against, since a count of complete rows means nothing without it.
//
// It counts and judges nothing. Whether a cell's verdict is right is the
// agent's; whether a cell exists at a `(unit, role)` for the unit's current hash
// is a fact about coverage.ndjson, and a row is complete exactly when §10.2.2
// says it is.
type Rows struct {
	// Units is how many units the round formed.
	Units int `json:"units"`
	// Complete is how many of them hold a cell for every active role.
	Complete int `json:"complete"`
	// Gaps is how many do not. Complete and Gaps always sum to Units.
	Gaps int `json:"gaps"`
	// Oversized is how many carry §3.4.5's flag, whatever their row holds.
	Oversized int `json:"oversized"`
	// Roles is how many active roles a complete row holds a cell for.
	Roles int `json:"active_roles"`
}

// RowsOf counts round's units against the cells filled in that round.
//
// units are the round's own, and active is §4.5.1's set as meta.json carries
// it. A cell counts toward a row under three conditions, each §10.2.2's or
// §9.3.5's rather than this function's: it was filled in round, since §3.4.6
// makes `u1` of an earlier round a different unit; it names an active role,
// since a row is complete for every active role and no other; and its
// `unit_hash` is the unit's current hash, since §10.2.2 asks for cells "filled
// for that unit's current unit hash" and a cell filled against older code is a
// gap however it reads.
func RowsOf(round int, units []unit.Unit, active []string, cells []Cell) Rows {
	filled := filledSeats(round, units, cells)
	rows := Rows{Units: len(units), Roles: len(active)}
	for i := range units {
		if units[i].Oversized {
			rows.Oversized++
		}
		if rowComplete(units[i].ID, active, filled) {
			rows.Complete++
		} else {
			rows.Gaps++
		}
	}
	return rows
}

// MissingSeats names every `(unit, role)` seat RowsOf counts as a gap, as
// `unit/role` in unit order and then in the active set's order, so §10.2.2's
// reason can say which cells to fill rather than only how many rows lack one.
//
// A seat whose round holds a cell filled for a hash the unit no longer carries
// is named with that said beside it: the cell exists and reads as filled in
// coverage.ndjson, and a reader told only that the seat is missing would look
// for a cell that is there.
func MissingSeats(round int, units []unit.Unit, active []string, cells []Cell) []string {
	filled := filledSeats(round, units, cells)
	earlier := make(map[cellKey]bool, len(cells))
	for i := range cells {
		if cells[i].Round == round {
			earlier[cellKey{unit: cells[i].Unit, role: cells[i].Role}] = true
		}
	}
	missing := make([]string, 0)
	for i := range units {
		for _, role := range active {
			seat := cellKey{unit: units[i].ID, role: role}
			if filled[seat] {
				continue
			}
			named := units[i].ID + "/" + role
			if earlier[seat] {
				named += " (filled for an earlier unit hash)"
			}
			missing = append(missing, named)
		}
	}
	return missing
}

// filledSeats is the seats holding a cell RowsOf counts: filled in round, for
// the unit's current hash.
func filledSeats(round int, units []unit.Unit, cells []Cell) map[cellKey]bool {
	current := make(map[string]string, len(units))
	for i := range units {
		current[units[i].ID] = units[i].Hash
	}
	filled := make(map[cellKey]bool, len(cells))
	for i := range cells {
		if cells[i].Round == round && cells[i].UnitHash == current[cells[i].Unit] {
			filled[cellKey{unit: cells[i].Unit, role: cells[i].Role}] = true
		}
	}
	return filled
}

// cellKey is §4.5.6's key: a cell sits at one `(unit, role)`.
type cellKey struct {
	unit, role string
}

// rowComplete reports whether a unit holds a counted cell for every active role.
func rowComplete(id string, active []string, filled map[cellKey]bool) bool {
	for _, role := range active {
		if !filled[cellKey{unit: id, role: role}] {
			return false
		}
	}
	return true
}
