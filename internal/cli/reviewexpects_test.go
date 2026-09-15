package cli

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/state"
)

// expectsTwoCells is a fan-out whose round owes two cells, one per role, for
// the single unit its prompts were emitted over. The convention cell is one
// coverage.ndjson already holds.
func expectsTwoCells() *reviewResult {
	return &reviewResult{Fanout: &review.Fanout{
		Round: 2, Head: "abc123",
		Prompts: []review.Prompt{
			{Role: "correctness", Axis: "correctness", Unit: "u1", Text: "# Correctness on u1\n"},
		},
		Honesty: []string{},
		Expected: []review.ExpectedCell{
			{Expected: coverage.Expected{Unit: "u1", Role: "correctness"}},
			{Expected: coverage.Expected{Unit: "u1", Role: "convention"}, Recorded: true},
		},
	}}
}

// `cr review` reports the set of cells it expects to be filled, in both of
// §12.1's shapes.
//
// The set is printed rather than counted, because a count says how many cells
// are owed and not which, and §4.6.3's reason for reporting it at all is that
// §10.2.2 is checked against it once the roles return. The two renderings are
// asserted together so a reader at a terminal and a caller reading the JSON
// cannot be told different things about what the round is waiting for —
// convention is in both, although no prompt below was emitted for it, and both
// say it is already recorded (field-feedback 1.6).
func TestAReviewReportsTheCellsItExpectsToBeFilled(t *testing.T) {
	var printed bytes.Buffer
	require.NoError(t, (&writer{out: &printed, mode: ModeText}).emit(expectsTwoCells()))

	text := printed.String()
	assert.Contains(t, text, "expects 2 cell(s) for §10.2.2")
	assert.Contains(t, strings.Split(text, "\n"), "  u1 correctness")
	assert.Contains(t, strings.Split(text, "\n"), "  u1 convention (recorded)")

	var piped bytes.Buffer
	require.NoError(t, (&writer{out: &piped, mode: ModeJSON}).emit(expectsTwoCells()))

	var payload struct {
		Expected []map[string]any `json:"expected_cells"`
	}
	require.NoError(t, json.Unmarshal(piped.Bytes(), &payload))
	assert.Equal(t, []map[string]any{
		{"unit": "u1", "role": "correctness"},
		{"unit": "u1", "role": "convention", "recorded": true},
	}, payload.Expected)
}

// `cr review` marks the expected cells coverage.ndjson holds for the round and
// head, and only those, and §4.6.1 withholds exactly those prompts unless
// `--all` is given (field-feedback 1.6).
//
// statusHome files a pass cell on u1 from every active role and none on u2, so
// the three u1 entries are marked and the three u2 entries are not; with the
// cells taken away, no entry is marked and every prompt is emitted.
func TestAReviewMarksTheExpectedCellsTheRoundHasRecorded(t *testing.T) {
	statusHome(t)

	withCells := fanoutOf(t)
	everyCell := fanoutOf(t, "--all")
	layout, err := state.Default()
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileCoverage, []byte{}))
	require.NoError(t, held.Unlock())
	withoutCells := fanoutOf(t)

	expected := func(recordedUnit string) []review.ExpectedCell {
		cells := make([]review.ExpectedCell, 0, 6)
		for _, unitID := range []string{"u1", "u2"} {
			for _, role := range []string{"convention", "correctness", "intent-coverage"} {
				cells = append(cells, review.ExpectedCell{
					Expected: coverage.Expected{Unit: unitID, Role: role}, Recorded: unitID == recordedUnit,
				})
			}
		}
		return cells
	}
	assert.Equal(t, expected("u1"), withCells.Expected)
	assert.Equal(t, expected(""), withoutCells.Expected)
	assert.Equal(t, []string{"u2", "u2", "u2"}, promptUnits(withCells.Prompts),
		"§4.6.1: the prompts of the three recorded cells are withheld")
	assert.Equal(t, []string{"u1", "u1", "u1", "u2", "u2", "u2"}, promptUnits(everyCell.Prompts),
		"§4.6.1: --all emits them")
	assert.Equal(t, []string{"u1", "u1", "u1", "u2", "u2", "u2"}, promptUnits(withoutCells.Prompts))
}

// promptUnits is the unit of every prompt emitted, sorted.
func promptUnits(prompts []review.Prompt) []string {
	units := make([]string, 0, len(prompts))
	for i := range prompts {
		units = append(units, prompts[i].Unit)
	}
	slices.Sort(units)
	return units
}
