package intent

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §3.3's `source` row closes the set at five, and the five are what §3.3.1
// partitions: three of them are validated against the text before §3.1.5's
// first separator line, the fourth against a note, and the fifth against the
// extra intent file it names. A sixth would be a claim no rule checks, so the
// vocabulary is asserted as a whole — the set, its order, and the refusal of
// anything outside it — rather than one value at a time.
func TestTheFiveClaimSourcesAreTheOnesTheSpecWrites(t *testing.T) {
	assert.Equal(t,
		[]ClaimSource{
			ClaimFromDescription, ClaimFromAcceptance, ClaimFromComment,
			ClaimFromNote, ClaimFromFile,
		},
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
		assert.Contains(t, err.Error(), "description, acceptance, comment, note, file",
			"and names the five the user may choose from")
	}

	widened := ClaimSources()
	widened[0] = ClaimFromNote
	assert.Equal(t, ClaimFromDescription, ClaimSources()[0],
		"the set is a copy, so a caller can neither widen it nor reorder it")
}

// A source crosses the wire as the name §3.3's row writes, and comes back
// through the one parser, so claims.ndjson can hold no value cr cannot act on.
//
// The two empty spellings are the interesting half. `source` is a required row,
// and null and "" are the two ways a line can hold the key and supply nothing
// under it: both leave the zero source in place rather than failing here, so
// the required-field walk names the row the agent omitted instead of the
// vocabulary it did not choose from.
func TestAClaimSourceCrossesTheWireAsItsName(t *testing.T) {
	for _, known := range ClaimSources() {
		line, err := json.Marshal(known)
		require.NoError(t, err)
		assert.JSONEq(t, strconv.Quote(known.String()), string(line))

		var read ClaimSource
		require.NoError(t, json.Unmarshal(line, &read))
		assert.Equal(t, known, read)
	}

	var refused ClaimSource
	var unknown *UnknownClaimSourceError
	require.ErrorAs(t, json.Unmarshal([]byte(`"spec"`), &refused), &unknown,
		"a name outside §3.3 is refused at the file")
	assert.Equal(t, ClaimSource{}, refused)

	require.Error(t, json.Unmarshal([]byte(`7`), &refused),
		"a source that is not a string is not a name either")

	for _, empty := range []string{`null`, `""`, `  null  `} {
		supplied := ClaimFromNote
		require.NoError(t, json.Unmarshal([]byte(empty), &supplied), empty)
		assert.Equal(t, ClaimFromNote, supplied,
			"%s supplies nothing, so §3.3's required-field walk is what answers it", empty)
	}
}
