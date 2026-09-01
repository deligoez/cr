package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// One unit's changed lines, before and after the edit that re-clusters it.
//
// The hashes the tests below compare are §3.4.6's own, taken over these lines
// by the internal/unit that `cr brief` runs, rather than two literals written
// down here. A test naming two hash strings would prove that two strings are
// unequal; this one proves that changed code gives a changed unit hash, which
// is the thing §10.2.2 rests on.
var (
	beforeTheEdit = []string{"$total = $items->sum('price');", "return $total;"}
	afterTheEdit  = []string{"$total = $items->sum('price') + $shipping;", "return $total;"}
)

// clusteredUnit is §3.4.4's cluster put through §3.4.6: one file, one
// RIGHT-side hunk, and the changed lines given. It is the same call `cr brief`
// makes, so the hash it produces cannot drift from the hash a round records.
func clusteredUnit(t *testing.T, changed []string) *unit.Unit {
	t.Helper()
	lines := make([]git.ChangedLine, 0, len(changed))
	for i, text := range changed {
		lines = append(lines, git.ChangedLine{Side: git.Right, Line: 12 + i, Text: text})
	}
	formed, err := unit.Units([]unit.Cluster{{
		Path:      "src/Order.php",
		Side:      git.Right,
		Formation: unit.ByFallback,
		Hunks: []git.Hunk{{
			Path:      "src/Order.php",
			BaseStart: 12, BaseLines: len(changed),
			HeadStart: 12, HeadLines: len(changed),
			Side:    git.Right,
			Changed: lines,
		}},
	}})
	require.NoError(t, err)
	require.Len(t, formed, 1)
	return &formed[0]
}

// briefedAtUnit writes what a round leaves behind: meta.json with two active
// roles, and units.ndjson holding the one unit given.
//
// Calling it a second time with a re-clustered unit is what a changed unit
// looks like on disk, and the head does not move with it. §3.4.6's hash is
// taken over the unit's changed lines, so a unit can be re-clustered under an
// unchanged head — which is exactly the drift §9.3.1's head comparison cannot
// see and §10.2.2's second condition exists for.
func briefedAtUnit(t *testing.T, layout state.Layout, formed *unit.Unit) {
	t.Helper()
	held, err := layout.LockPR(cellsOwner, cellsRepo, cellsPR)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: cellsOwner, Repo: cellsRepo, PR: cellsPR,
		IssueKey: "CR-7", Round: 1, Head: cellsHead,
		ActiveRoles: []string{"convention", "correctness"},
	}))
	require.NoError(t, state.WriteStamped(held, state.FileUnits,
		state.Stamp{Head: cellsHead, Round: 1}, []*unit.Record{{Unit: *formed}}))
	require.NoError(t, held.Unlock())
}

// briefedForComputedCells opens the state tree and puts one round in it.
func briefedForComputedCells(t *testing.T, formed *unit.Unit) state.Layout {
	t.Helper()
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR(cellsOwner, cellsRepo, cellsPR))
	briefedAtUnit(t, layout, formed)
	return layout
}

// theRecordedCell is the round's one coverage cell, read back off disk.
func theRecordedCell(t *testing.T, layout state.Layout) coverage.Cell {
	t.Helper()
	stored, err := state.ReadRecords[coverage.Cell](
		layout, cellsOwner, cellsRepo, cellsPR, state.FileCoverage)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	return stored[0]
}

// The rejection of a computed field reaches the shell as §11.2's code 1, and
// the file that was refused leaves the round exactly as it found it.
//
// §4.5.5's two computed fields are refused by different guards — `head` by
// state.DecodeStamped, which owns §2.3.3's pair for all eight files, and
// `unit_hash` by the cell's own decoder — and both are asserted here through
// the command, because what a caller sees is one exit code and one round on
// disk. A guard that refused only inside its package, or that returned a usage
// error, would tell the agent to retype a command line that was right.
func TestACellSupplyingAFieldCrComputesExitsOne(t *testing.T) {
	formed := clusteredUnit(t, beforeTheEdit)
	layout := briefedForComputedCells(t, formed)
	require.NoError(t, recordCells(t, `{"unit":"u1","role":"correctness","result":"pass"}`))

	for _, tc := range []struct {
		name string
		line string
	}{
		{
			name: "the unit hash it would have been checked against",
			line: `{"unit":"u1","role":"convention","result":"pass","unit_hash":"` +
				formed.Hash + `"}`,
		},
		{
			name: "a unit hash that is no unit's",
			line: `{"unit":"u1","role":"convention","result":"pass","unit_hash":"0000000000000000"}`,
		},
		{
			name: "the head it was filled against",
			line: `{"unit":"u1","role":"convention","result":"pass","head":"` + cellsHead + `"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := recordCells(t, tc.line)

			require.Error(t, err)
			assert.Equal(t, ExitValidation, exitCodeFor(err),
				"§6.1.4 and §11.2 code a supplied computed field 1")
			assert.Equal(t, []string{"u1/correctness"}, filledCells(t, layout),
				"and the refused file leaves the round as it found it")
		})
	}
}

