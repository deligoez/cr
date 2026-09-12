package mapping

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// The round's ids every decode below is checked against: what §3.3.1 recorded
// and what §3.7 had `cr brief` write.
var (
	roundClaims = []string{"CR-1#c1", "CR-1#c2"}
	roundUnits  = []string{"u1", "u2"}
)

// rejected decodes body against the round's ids and returns the refusal, which
// the test requires to be §4.1.6's and to have handed back no pairs.
func rejected(t *testing.T, body string, claims, units []string) *RejectedPairError {
	t.Helper()
	pairs, err := Decode("mapping.ndjson", []byte(body), claims, units)
	assert.Nil(t, pairs, "a refused mapping gives the caller nothing to write")
	var refusal *RejectedPairError
	require.ErrorAs(t, err, &refusal)
	return refusal
}

// A mapping whose every line names a claim and a unit the round holds decodes
// to its pairs, in the order the agent wrote them, blank lines skipped.
//
// The order is the agent's, which ClaimsOf hands on to §4.6.1's prompt, and a
// claim may be mapped to several units and a unit to several claims (§4.1.1):
// neither is a repeat to refuse.
func TestDecodeReturnsEveryPairOfAWellFormedMappingInOrder(t *testing.T) {
	body := `{"claim":"CR-1#c2","unit":"u1"}

{"claim":"CR-1#c1","unit":"u1"}
{"claim":"CR-1#c1","unit":"u2"}
`

	pairs, err := Decode("mapping.ndjson", []byte(body), roundClaims, roundUnits)

	require.NoError(t, err)
	got := make([]string, 0, len(pairs))
	for _, p := range pairs {
		got = append(got, p.Claim+"->"+p.Unit)
	}
	assert.Equal(t, []string{"CR-1#c2->u1", "CR-1#c1->u1", "CR-1#c1->u2"}, got)
}

// An empty mapping is a round the agent mapped nothing in, and decodes to an
// empty list rather than a nil one (§12).
func TestDecodeOfAnEmptyMappingIsAnEmptyList(t *testing.T) {
	pairs, err := Decode("mapping.ndjson", []byte("\n\n"), roundClaims, roundUnits)

	require.NoError(t, err)
	assert.NotNil(t, pairs)
	assert.Empty(t, pairs)
}

// A claim or unit that is absent, null, or the empty string is refused as
// missing rather than as unknown.
//
// The three spellings are one case to §4.1.6, and the refusal says the field
// is required. Reading null or "" as a supplied id would reach the unknown-id
// branch instead and tell the agent its id is not the round's, when it wrote
// no id at all.
func TestDecodeRefusesAMissingIdInEverySpellingAsRequired(t *testing.T) {
	cases := map[string]struct {
		body  string
		field string
	}{
		"absent claim": {`{"unit":"u1"}`, "claim"},
		"null claim":   {`{"claim":null,"unit":"u1"}`, "claim"},
		"empty claim":  {`{"claim":"","unit":"u1"}`, "claim"},
		"absent unit":  {`{"claim":"CR-1#c1"}`, "unit"},
		"null unit":    {`{"claim":"CR-1#c1","unit":null}`, "unit"},
		"empty unit":   {`{"claim":"CR-1#c1","unit":""}`, "unit"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			refusal := rejected(t, tc.body, roundClaims, roundUnits)

			assert.Equal(t, tc.field, refusal.Field)
			assert.Equal(t, 1, refusal.Line)
			assert.Contains(t, refusal.Problem, "is required by §4.1.6")
		})
	}
}

// An id the round does not hold is refused on the line that carries it, and
// the line count includes the blank lines before it.
//
// The refusal names the id and lists what the round holds, so the agent can
// see which of its ids went stale without opening the round's files.
func TestDecodeRefusesAnUnknownIdOnItsLineNamingWhatTheRoundHolds(t *testing.T) {
	cases := map[string]struct {
		body  string
		field string
		named string
		holds string
	}{
		"unknown claim": {
			"{\"claim\":\"CR-1#c1\",\"unit\":\"u1\"}\n\n{\"claim\":\"CR-1#c9\",\"unit\":\"u1\"}\n",
			"claim", `"CR-1#c9"`, "CR-1#c1, CR-1#c2",
		},
		"unknown unit": {
			"{\"claim\":\"CR-1#c1\",\"unit\":\"u1\"}\n\n{\"claim\":\"CR-1#c1\",\"unit\":\"u7\"}\n",
			"unit", `"u7"`, "u1, u2",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			refusal := rejected(t, tc.body, roundClaims, roundUnits)

			assert.Equal(t, "mapping.ndjson", refusal.File)
			assert.Equal(t, 3, refusal.Line, "the blank line before the pair is counted")
			assert.Equal(t, tc.field, refusal.Field)
			assert.Contains(t, refusal.Problem, "names "+tc.named)
			assert.Contains(t, refusal.Problem, "the round holds "+tc.holds)
		})
	}
}

// A round that holds no ids of a kind says so, rather than trailing off after
// "holds".
func TestAnUnknownIdInARoundHoldingNoneSaysNone(t *testing.T) {
	refusal := rejected(t, `{"claim":"CR-1#c1","unit":"u1"}`, nil, roundUnits)

	assert.Equal(t, "claim", refusal.Field)
	assert.Contains(t, refusal.Problem, "the round holds none")
}

// A line with both ids wrong is always reported by its claim, the order
// §4.1.6 writes the pair in.
func TestALineWithBothIdsWrongIsReportedByItsClaim(t *testing.T) {
	refusal := rejected(t, `{"claim":"CR-1#c9","unit":"u9"}`, roundClaims, roundUnits)

	assert.Equal(t, "claim", refusal.Field)
}

// The refusal renders as the file, the line, the field, and the problem, which
// is what the cli layer prints under §12.4.
func TestARejectedPairNamesFileLineAndField(t *testing.T) {
	err := &RejectedPairError{File: "m.ndjson", Line: 4, Field: "unit", Problem: "is wrong"}

	assert.Equal(t, "m.ndjson line 4: unit is wrong", err.Error())
}

// The stamp is §2.3.3's and never the agent's: a pair that supplies head or
// round is refused through the shared decode, not accepted as written.
func TestDecodeRefusesAPairThatSuppliesItsOwnStamp(t *testing.T) {
	_, err := Decode("mapping.ndjson", []byte(`{"claim":"CR-1#c1","unit":"u1","round":1}`),
		roundClaims, roundUnits)

	var reserved *state.ReservedFieldError
	require.ErrorAs(t, err, &reserved)
	assert.Equal(t, "round", reserved.Field)
}
