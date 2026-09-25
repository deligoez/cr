package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §4.6.2 through `cr review`: every prompt names the NDJSON path its role
// writes its coverage cell to, `fanout/<round>/<unit>/cells-<role>.ndjson`
// beside the role's records, and the round's contract file carries §4.5.5's
// cell schema.
//
// Found on tarfin-labs/backend#6328 with cr 0.13.0: the prompts named a path
// for records and one for proposals and none for the cell, and contract.md
// held no cell schema.
func TestEveryPromptNamesItsCellPathAndTheContractCarriesTheCellSchema(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))

	prompts := fanoutOf(t, "--all").Prompts
	require.NotEmpty(t, prompts)
	for _, prompt := range prompts {
		want := filepath.Join(layout.FanOutDir(fixtureOwner, fixtureProject, fixturePRNumber, 1, prompt.Unit),
			"cells-"+prompt.Role+".ndjson")
		assert.Equal(t, want, prompt.Cells, "%s on %s", prompt.Role, prompt.Unit)
		assert.Contains(t, prompt.Text, "## Coverage cell (§4.5.5)\n\n"+
			"Write this role's coverage cell for this unit, one JSON object on one line, to:\n\n    "+want+"\n")
	}

	contract, err := os.ReadFile(layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileContract))
	require.NoError(t, err)
	assert.Contains(t, string(contract), "## Coverage cell schema (§4.5.5)\n\n"+
		"A cell carries unit, role, result, reason, note_id, coverage; cr writes unit_hash, head and round "+
		"itself, and a cell supplying one is rejected with exit code 1.\n"+
		`- result: one of "pass", "finding", "question", "na"`+"\n"+
		`- reason: required when result is "na", and refused on any other result`+"\n"+
		`- coverage: required on a cell a role on the test axis filled unless its result is "na", and `+
		`refused on every other cell; an object carrying classification, one of "covered", `+
		`"partially-covered", "uncovered", and test_paths, the test files the classification rests on (§4.4.1)`+"\n"+
		"- note_id: optional; the note that explains an unmapped unit (§4.1.5)\n")
}
