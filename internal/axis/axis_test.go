package axis

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
