package coverage

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// filledAt is a cell as coverage.ndjson holds it after `cr cells record`
// stamped it: the round it was filled in and the unit hash it was filled for.
func filledAt(round int, unitID, role, hash string) Cell {
	return Cell{
		Unit: unitID, Role: role, Result: ResultPass, UnitHash: hash,
		Stamp: state.Stamp{Round: round},
	}
}

// §10.1.1's four unit counts over a round of three units and two active roles.
//
// Each unit fails or passes for its own reason, so every condition RowsOf
// counts a cell under has a unit that turns on it alone: u1 holds both cells;
// u2 holds one, and a second at a role the round did not activate; u3 holds
// both roles, but one cell was filled for a hash the unit no longer carries and
// the other in an earlier round, where u3 was a different unit.
func TestRowsCountCompleteRowsAndGapsAsSection1011Does(t *testing.T) {
	units := []unit.Unit{
		{ID: "u1", Hash: "h1"},
		{ID: "u2", Hash: "h2", Oversized: true},
		{ID: "u3", Hash: "h3"},
	}
	active := []string{"correctness", "convention"}
	cells := []Cell{
		filledAt(2, "u1", "correctness", "h1"),
		filledAt(2, "u1", "convention", "h1"),
		filledAt(2, "u2", "correctness", "h2"),
		filledAt(2, "u2", "security", "h2"),
		filledAt(2, "u3", "correctness", "stale"),
		filledAt(1, "u3", "convention", "h3"),
	}

	assert.Equal(t, Rows{Units: 3, Complete: 1, Gaps: 2, Oversized: 1, Roles: 2},
		RowsOf(2, units, active, cells))
}

// §10.2.2 asks for a complete row for every active role, and a round with no
// active role asks for nothing. The count of roles travels with the count of
// rows so that answer is never read as coverage without the zero beside it.
func TestWithNoActiveRoleEveryRowIsCompleteAgainstZeroRoles(t *testing.T) {
	rows := RowsOf(1, []unit.Unit{{ID: "u1", Hash: "h1"}}, []string{}, []Cell{})

	assert.Equal(t, Rows{Units: 1, Complete: 1, Roles: 0}, rows)
}
