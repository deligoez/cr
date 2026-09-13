package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
)

// §4.2 over the correctness axis: u1 is mapped to CR claim c1 and u2, the test
// file's unit, to none. Both still produce a correctness prompt (§4.2.3), u1's
// carries the claim it is evaluated against (§4.2.1) and the obligation to cite
// it (§4.2.2), and u2's tells the role the unit is still evaluated rather than
// leaving an empty claims section to read as nothing to do.
func TestAZeroClaimUnitStillProducesACorrectnessPrompt(t *testing.T) {
	src := briefed(t)
	src.Axis = axis.Correctness

	fan, err := Run(src)
	require.NoError(t, err)

	units := make([]string, 0, len(fan.Prompts))
	for _, prompt := range fan.Prompts {
		assert.Equal(t, axis.Correctness, prompt.Axis)
		units = append(units, prompt.Unit)
	}
	assert.Equal(t, []string{"u1", "u2"}, units, "§4.2.3: the unit mapped to zero claims is evaluated too")

	mapped := promptOf(t, fan, "correctness", "u1")
	assert.Contains(t, mapped, "- "+runIssue+"#c1: ", "§4.2.1: the claim the unit is mapped to")
	assert.Contains(t, mapped, "cites that claim's id in claim (§4.2.2)")

	unmapped := promptOf(t, fan, "correctness", "u2")
	assert.Contains(t, unmapped, "The mapping maps no claim to this unit.")
	assert.Contains(t, unmapped, "No claim is mapped to this unit, and it is still evaluated (§4.2.3)")
}
