package intent

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
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

// A span can stop occurring while the issue text reports no drift, and the
// report says so.
//
// The two questions §3.3.3 asks are asked of different texts. The hash is taken
// over the §1.4 normal form, whose fourth step replaces every run of spaces and
// tabs with a single space, so doubling a space between two words leaves the
// hash exactly as it was. The span is "the verbatim substring of the source
// text" per §3.3's table, and §3.3.1 refuses a claim by a literal comparison,
// so the same edit takes the span away.
//
// That is why SpanOccurs is computed for every claim rather than only for the
// drifted ones. A report that asked the question only when the hashes differed
// would answer "no drift" here and say nothing about the claim that can no
// longer be pointed at anything in the issue.
func TestASpanCanGoWhileTheHashStandsStill(t *testing.T) {
	stored := extracted(t)
	spaced := "The upload retries on a 5xx response.\n" +
		"The upload is abandoned after  five attempts.\n"

	drift, err := DetectDrift(slices.Values(stored), spaced)
	require.NoError(t, err)

	assert.False(t, drift.Drifted,
		"§1.4 step 4 collapses the doubled space, so the normalised hash did not move")
	assert.Equal(t, stored[1].IssueHash, drift.Hash)
	assert.True(t, drift.Claims[0].SpanOccurs, "the untouched line still holds its span")
	assert.False(t, drift.Claims[1].SpanOccurs,
		"§3.3 calls a span verbatim, and the doubled space is not in the stored span")
}

// §3.3.3's "cr MUST NOT re-extract", read off the source rather than off a
// comment: the file that detects drift cannot write per-PR state, because it
// does not import the package that holds the only door to it.
//
// §2.3.1 admits no unlocked write to §2.3's files, and state.Lock is how that
// is enforced — Write is a method on the held lock, so a file with no
// internal/state import has no way to reach claims.ndjson at all. That is the
// half of the invariant a signature cannot express: DetectDrift's parameters
// say it is not *given* a lock, and this says it cannot go and take one.
//
// The check is on imports rather than on the text of the file, for the reason
// internal/cli's runner fence gives: an import is what a call needs and cannot
// be spelled around, so a package cannot be reached by aliasing it or by
// building its name at run time.
func TestDriftDetectionCannotReachThePerPRState(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "drift.go", nil, parser.ImportsOnly)
	require.NoError(t, err)

	imported := make([]string, 0, len(parsed.Imports))
	for _, spec := range parsed.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		require.NoError(t, err)
		imported = append(imported, path)
	}
	require.NotEmpty(t, imported, "a fence over no imports at all would pass on an empty file")
	assert.NotContains(t, imported, "github.com/deligoez/cr/internal/state",
		"§3.3.3 has drift detection report and nothing else, and state.Lock is the only "+
			"way to write the claims it reports on")
}
