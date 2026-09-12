package coverage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The set is every unit paired with every active role, unit by unit and role by
// role within each, and nothing is left out of it.
func TestTheExpectedSetIsTheCrossProductInTheRoundsOwnOrder(t *testing.T) {
	assert.Equal(t, []Expected{
		{Unit: "u1", Role: "correctness"},
		{Unit: "u1", Role: "convention"},
		{Unit: "u2", Role: "correctness"},
		{Unit: "u2", Role: "convention"},
	}, Expect([]string{"u1", "u2"}, []string{"correctness", "convention"}))
}

// A round with no unit, or with no active role, expects no cell — and says so
// with an empty set rather than a nil one, per §12.3.
func TestARoundMissingEitherSideExpectsAnEmptySetNotANilOne(t *testing.T) {
	for name, expected := range map[string][]Expected{
		"no unit":        Expect(nil, []string{"correctness"}),
		"no active role": Expect([]string{"u1"}, nil),
	} {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, expected)
			assert.Empty(t, expected)
		})
	}
}
