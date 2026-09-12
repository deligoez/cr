package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/review"
)

// expectsTwoCells is a fan-out whose round owes two cells, one per role, for
// the single unit its prompts were emitted over.
func expectsTwoCells() *reviewResult {
	return &reviewResult{Fanout: &review.Fanout{
		Round: 2, Head: "abc123",
		Prompts: []review.Prompt{
			{Role: "correctness", Axis: "correctness", Unit: "u1", Text: "# Correctness on u1\n"},
		},
		Honesty: []string{},
		Expected: []coverage.Expected{
			{Unit: "u1", Role: "correctness"},
			{Unit: "u1", Role: "convention"},
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
// convention is in both, although no prompt below was emitted for it.
func TestAReviewReportsTheCellsItExpectsToBeFilled(t *testing.T) {
	var printed bytes.Buffer
	require.NoError(t, (&writer{out: &printed, mode: ModeText}).emit(expectsTwoCells()))

	text := printed.String()
	assert.Contains(t, text, "expects 2 cell(s) for §10.2.2")
	assert.Contains(t, text, "\n  u1 correctness\n")
	assert.Contains(t, text, "\n  u1 convention\n")

	var piped bytes.Buffer
	require.NoError(t, (&writer{out: &piped, mode: ModeJSON}).emit(expectsTwoCells()))

	var payload struct {
		Expected []coverage.Expected `json:"expected_cells"`
	}
	require.NoError(t, json.Unmarshal(piped.Bytes(), &payload))
	assert.Equal(t, []coverage.Expected{
		{Unit: "u1", Role: "correctness"},
		{Unit: "u1", Role: "convention"},
	}, payload.Expected)
}
