package coverage

// Expected is one cell §10.2.2 demands of a round: the `(unit, role)` at which
// a cell must stand before the round can be called complete.
//
// It is not a Cell and carries none of §4.5.5's fields. A cell records what a
// role found; this records only that the round is waiting for one, and the
// difference is the whole point of §4.6.3 — the set is known before any role
// has looked, which is what lets a caller check §10.2.2 once they return
// instead of discovering afterwards which of them never reported.
type Expected struct {
	// Unit is the unit id of §3.4.6.
	Unit string `json:"unit"`
	// Role is the active role of §4.5.1 expected to fill the cell.
	Role string `json:"role"`
}

// Expect is §4.6.3's set: every unit of the round paired with every active
// role, unit by unit in §3.4.6's id order and role by role in §2.5.5's corpus
// order within each.
//
// Nothing is subtracted. §10.2.2 counts a row complete for every active role,
// so a role §4.6.4 reports as skipped is still expected — the round is
// incomplete because it did not look, and dropping its cells from the set would
// make the round read as complete on the strength of a lens that never ran.
// The same holds for an oversized unit, for a unit mapped to no claim, and for
// the axis one invocation happened to narrow the prompts to: none of them
// changes what §10.2.2 demands of the round, and this is a statement about the
// round rather than about the invocation.
//
// The order is the two orders the round already has, so a caller comparing two
// runs of the same round compares two identical lists (§2.1.1).
func Expect(units, active []string) []Expected {
	// The capacity is a product of lengths, never a difference: `make`
	// panics on a negative capacity, and len is non-negative for every
	// slice, the nil one included, so this product cannot reach one.
	expected := make([]Expected, 0, len(units)*len(active))
	for _, id := range units {
		for _, lens := range active {
			expected = append(expected, Expected{Unit: id, Role: lens})
		}
	}
	return expected
}
