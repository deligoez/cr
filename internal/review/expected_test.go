package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// unitsOfRound is the round's units as §10.2.2 reads them, off units.ndjson,
// carrying the current unit hash a cell is counted against.
func unitsOfRound(t *testing.T, src *Sources) []unit.Unit {
	t.Helper()
	records, err := state.ReadRecords[unit.Record](src.Layout, runOwner, runRepo, runPR, state.FileUnits)
	require.NoError(t, err)
	units := make([]unit.Unit, 0, len(records))
	for i := range records {
		units = append(units, records[i].Unit)
	}
	return units
}

// filling is the cell a role would write for each expected `(unit, role)`,
// stamped as `cr cells record` stamps one: the round's head and round, and the
// unit's current hash per §4.5.6.
//
// The verdict is `pass` because §10.2.2 counts a cell's existence and not what
// it says; what is under test is whether the reported set is the set that
// closes every row.
func filling(expected []coverage.Expected, units []unit.Unit, round int, head string) []coverage.Cell {
	hashes := make(map[string]string, len(units))
	for i := range units {
		hashes[units[i].ID] = units[i].Hash
	}
	cells := make([]coverage.Cell, 0, len(expected))
	for _, at := range expected {
		cells = append(cells, coverage.Cell{
			Unit: at.Unit, Role: at.Role, Result: coverage.ResultPass,
			UnitHash: hashes[at.Unit], Stamp: state.Stamp{Head: head, Round: round},
		})
	}
	return cells
}

// The cells `cr review` says it expects are exactly the cells §10.2.2 demands
// of the same round.
//
// The comparison is made through coverage.RowsOf rather than against a list
// written out here, because RowsOf is where §10.2.2 is implemented: filling
// exactly the reported set has to close every row, and the two halves of that
// sentence are both asserted — the full set leaves no gap, and the set one cell
// short leaves exactly one. A reported set that was too small would pass the
// first assertion of a test that only filled what it was told to and counted
// the result, so the short fill is what gives this teeth.
func TestTheExpectedSetIsExactlyWhatCompletenessDemands(t *testing.T) {
	src := briefed(t)
	fan, err := Run(src)
	require.NoError(t, err)

	meta, err := src.Layout.Briefed(runOwner, runRepo, runPR, src.currentHead)
	require.NoError(t, err)
	units := unitsOfRound(t, src)
	require.Len(t, fan.Expected, len(units)*len(meta.ActiveRoles),
		"§4.6.3: the set is the cross product, so it is as large as one")

	complete := coverage.RowsOf(fan.Round, units, meta.ActiveRoles,
		filling(fan.Expected, units, fan.Round, fan.Head))
	assert.Equal(t, coverage.Rows{
		Units: len(units), Complete: len(units), Gaps: 0, Roles: len(meta.ActiveRoles),
	}, complete, "§10.2.2: filling the reported set closes every row")

	short := coverage.RowsOf(fan.Round, units, meta.ActiveRoles,
		filling(fan.Expected[1:], units, fan.Round, fan.Head))
	assert.Equal(t, 1, short.Gaps,
		"one cell of the reported set left unfilled is one row §10.2.2 refuses")
}

// The expected set is the cross product of the round's active roles and its
// units, minus nothing — including on an `--axis` pass that emits a fraction of
// the prompts.
//
// §4.6.5 runs the fan-out in two passes, and §10.2.2 does not know about either:
// a round whose intent pass reported only the intent cells would read as
// complete once those came back, on the strength of three axes that had not run.
// So the set is a statement about the round and the prompts are a statement
// about the invocation.
func TestTheExpectedSetIsTheWholeRoundsEvenOnAnAxisPass(t *testing.T) {
	src := briefed(t)
	src.Axis = "intent"

	fan, err := Run(src)
	require.NoError(t, err)

	require.Len(t, fan.Prompts, 2, "the intent pass emits its own axis's prompts alone")
	at := make([]string, 0, len(fan.Expected))
	for _, cell := range fan.Expected {
		at = append(at, cell.Unit+"/"+cell.Role)
	}
	assert.Equal(t, []string{
		"u1/convention", "u1/correctness", "u1/intent-coverage", "u1/test-adequacy",
		"u2/convention", "u2/correctness", "u2/intent-coverage", "u2/test-adequacy",
	}, at, "§4.6.3: every active role over every unit, whatever this pass emitted")
}
