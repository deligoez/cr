package intent

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/text"
)

// The issue text before and after the round that moved it. The first line is
// untouched and the second is reworded, so one claim's span survives and the
// other's does not — a mutation that changed everything or nothing would let a
// report that answers the same for every claim pass.
const (
	issueBefore = "The upload retries on a 5xx response.\n" +
		"The upload is abandoned after five attempts.\n"
	issueAfter = "The upload retries on a 5xx response.\n" +
		"The upload is abandoned after three attempts.\n"
)

// extracted is the round's claims, decoded and hashed the way `cr claims
// record` writes them, so the `issue_hash` §3.3.3 compares against is the one
// cr computed rather than one the test wrote.
func extracted(t *testing.T) []Claim {
	t.Helper()
	spans := SpanTexts{Issue: issueBefore}
	decoded, err := DecodeClaims(claimsFile, []byte(
		`{"id":"CR-1#c1","text":"Retry a 5xx.","source":"acceptance",`+
			`"span":"retries on a 5xx"}`+"\n"+
			`{"id":"CR-1#c2","text":"Give up after five.","source":"acceptance",`+
			`"span":"abandoned after five attempts"}`+"\n",
	), "CR-1", spans)
	require.NoError(t, err)
	require.NoError(t, ComputeClaimHashes(decoded, issueBefore))

	stored := make([]Claim, 0, len(decoded))
	for _, claim := range decoded {
		stored = append(stored, *claim)
	}
	return stored
}

// asStored renders claims as the NDJSON claims.ndjson holds, so "no claim
// changed" is asserted on bytes rather than on a field somebody remembered to
// check.
func asStored(t *testing.T, claims []Claim) string {
	t.Helper()
	encoded, err := json.Marshal(claims)
	require.NoError(t, err)
	return string(encoded)
}

// §3.3.3 through a moved issue: the hashes differ, and every claim is reported
// with whether its span still occurs — while not one claim changes.
//
// The last clause is the point of the test and is asserted on the stored bytes.
// §3.3.3's own sentence is "cr MUST NOT re-extract", and a report that quietly
// refreshed a span, a hash, or an id would satisfy every other assertion here:
// the drift would be reported correctly once and the claims would then agree
// with the new text, so the next round would find no drift at all and the
// change would never surface.
func TestDriftReportsEachClaimAndChangesNone(t *testing.T) {
	stored := extracted(t)
	before := asStored(t, stored)

	drift, err := DetectDrift(slices.Values(stored), issueAfter)
	require.NoError(t, err)

	after, err := text.NormalisedHash(issueAfter)
	require.NoError(t, err)
	assert.Equal(t, after, drift.Hash,
		"§3.3.3 re-reads the issue text and hashes it per §1.4")
	assert.True(t, drift.Drifted,
		"the stored issue_hash and the fresh one differ, which is what drift is")

	require.Len(t, drift.Claims, 2, "§3.3.3 reports for each claim, so every claim is in it")
	assert.Equal(t, []ClaimDrift{
		{ID: "CR-1#c1", ExtractedHash: stored[0].IssueHash, SpanOccurs: true},
		{ID: "CR-1#c2", ExtractedHash: stored[1].IssueHash, SpanOccurs: false},
	}, drift.Claims,
		"the first claim's span survived the rewording and the second's did not")
	for _, reported := range drift.Claims {
		assert.True(t, reported.Drifted(drift.Hash),
			"both claims were extracted against the text that moved")
	}

	assert.Equal(t, before, asStored(t, stored),
		"§3.3.3: cr MUST NOT re-extract, so the claims come out byte for byte as they went in")
}

// An issue text that has not moved reports no drift, and every span is still
// where it was.
//
// It is the case §3.3.3 runs on most rounds, and the one a report that always
// answered "drifted" would get wrong while passing the mutation test above.
// The claims are checked byte for byte here too: a round that changes nothing
// is exactly where an unnecessary rewrite would go unnoticed.
func TestAnUnmovedIssueTextReportsNoDrift(t *testing.T) {
	stored := extracted(t)
	before := asStored(t, stored)

	drift, err := DetectDrift(slices.Values(stored), issueBefore)
	require.NoError(t, err)

	assert.False(t, drift.Drifted, "the stored issue_hash and the fresh one agree")
	assert.Equal(t, stored[0].IssueHash, drift.Hash,
		"and the fresh hash is the one the extraction stored")
	for _, reported := range drift.Claims {
		assert.True(t, reported.SpanOccurs, reported.ID)
		assert.False(t, reported.Drifted(drift.Hash), reported.ID)
	}

	assert.Equal(t, before, asStored(t, stored), "and nothing was rewritten")
}
