package role

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.5 gives cr the output contract and leaves a role persona and focus, and a
// role file is project-owned and hand-edited — untrusted input by construction.
// The fault worth defending against is therefore not a typo but a role that
// redefines what cr emits: an `output_path` beside the one §4.6.2 names, a
// `schema` beside §6.1's, or a §6.1.4 stamp field the role would like to write
// itself.
//
// Ignoring such a key is the wrong answer twice over. It leaves the author
// believing the key took effect, which is the failure with a cost — a role that
// thinks it redirected its output writes nowhere and reports nothing. And it
// makes the boundary a convention rather than a fence: with unknown keys
// tolerated, no reader can tell a supported field from a decorative one, and
// the next field someone adds is discovered only when it silently does nothing.
// So an unknown top-level key aborts on §2.5.3's exit code, naming the key.
//
// TestRoleCarriesExactlyTheSpecFields is the other half. That one fixes what
// the struct may hold; this one fixes what a file may say. Neither implies the
// other: a struct held to the table still decodes an unknown key into nothing,
// and an allowlist over a struct that grew a field would let it through.
func TestARoleFileCannotCarryAKeyCrDoesNotDefine(t *testing.T) {
	for _, key := range []string{"output_path", "schema", "record", "grade", "state", "prompt"} {
		t.Run(key, func(t *testing.T) {
			path := roleFile(t, "correctness", map[string]any{key: "mine.ndjson"})

			_, err := Load(path)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, key, malformed.Field)
			assert.Contains(t, err.Error(), path)
			assert.Contains(t, err.Error(), "cr owns the output contract")
		})
	}

	// A file carrying two unknown keys must report the same one on every run.
	// The map that finds them iterates in no order, so without the sort this
	// would pass most of the time and fail the rest — the shape of flake that
	// gets rerun rather than read.
	t.Run("two unknown keys report the same one every run", func(t *testing.T) {
		path := roleFile(t, "correctness", map[string]any{"zeta": "1", "alpha": "2"})

		for range 20 {
			_, err := Load(path)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, "alpha", malformed.Field)
		}
	})
}
