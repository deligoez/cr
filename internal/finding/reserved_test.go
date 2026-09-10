package finding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Reserved hands out the very fence the decoder refuses by, and a copy of it, so
// a caller stating the fence in a prompt can neither drift from it nor widen it.
func TestReservedHandsOutTheDecodersFenceAsACopy(t *testing.T) {
	handed := Reserved()
	assert.Equal(t, []string{
		"axis", "grade", "state", "disposition", "duplicate_of", "thread_id", "span_hash", "issue_hash",
	}, handed)

	handed[0] = "widened"
	assert.Equal(t, "axis", Reserved()[0], "editing the copy leaves the fence alone")
}
