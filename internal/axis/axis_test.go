package axis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spec/0.1.0.md §1.5 names the four axes and no others, so the set itself is
// asserted here rather than only the acceptance of each member.
func TestValidateAcceptsExactlyTheFourAxisIDs(t *testing.T) {
	assert.Equal(t, []string{"intent", "correctness", "convention", "test"}, IDs())
	for _, id := range IDs() {
		assert.True(t, Valid(id), "%s must be a valid axis id", id)
		assert.NoError(t, Validate("roles/some-role.json", "axis", id))
	}
}

// §1.5 aborts on any other value, and the abort is only actionable if it says
// which file carried it and what it said, so both are asserted on the error and
// in the message the user reads.
func TestValidateNamesTheOffendingFileAndValue(t *testing.T) {
	err := Validate("roles/security.json", "axis", "security")
	require.Error(t, err)

	var invalid *InvalidError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, "roles/security.json", invalid.File)
	assert.Equal(t, "axis", invalid.Field)
	assert.Equal(t, "security", invalid.Value)

	assert.Contains(t, err.Error(), "roles/security.json")
	assert.Contains(t, err.Error(), `"security"`)
	assert.Contains(t, err.Error(), "intent, correctness, convention, test")
}
