package intent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §3.3's `source` row closes the set at four, and the four are what §3.3.1 and
// §3.3.2 partition: three of them are validated against the issue text and the
// fourth against a note. A fifth would be a claim neither rule checks, so the
// vocabulary is asserted as a whole — the set, its order, and the refusal of
// anything outside it — rather than one value at a time.
func TestTheFourClaimSourcesAreTheOnesTheSpecWrites(t *testing.T) {
	assert.Equal(t,
		[]ClaimSource{ClaimFromDescription, ClaimFromAcceptance, ClaimFromComment, ClaimFromNote},
		ClaimSources())

	for _, known := range ClaimSources() {
		parsed, err := ParseClaimSource(known.String())
		require.NoError(t, err)
		assert.Equal(t, known, parsed, "%s parses back to itself", known)
	}

	for _, name := range []string{"", "spec", "Description", "description ", "issue"} {
		_, err := ParseClaimSource(name)
		var unknown *UnknownClaimSourceError
		require.ErrorAs(t, err, &unknown, "%q names no source of §3.3", name)
		assert.Equal(t, name, unknown.Value, "the refusal shows what was rejected")
		assert.Contains(t, err.Error(), "description, acceptance, comment, note",
			"and names the four the user may choose from")
	}

	widened := ClaimSources()
	widened[0] = ClaimFromNote
	assert.Equal(t, ClaimFromDescription, ClaimSources()[0],
		"the set is a copy, so a caller can neither widen it nor reorder it")
}
