package intent

import (
	"testing"

	"github.com/deligoez/cr/internal/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §3.3's table states both pre-images, and this is what they are worth if cr
// picks either of them differently. `span_hash` is the "Normalised hash of
// `span`" — the claim's own span, not its `text`, which is the agent's wording
// of the same thing and would hash to something else. `issue_hash` is the
// "Normalised hash of the whole issue text at extraction" — one value for the
// extraction, not one per claim, because §3.3.3 compares a fresh hash of the
// issue against "the stored `issue_hash`", singular.
//
// Both go through text.NormalisedHash, which runs §1.4's six steps on its input
// itself, so a span that differs from another only by trailing whitespace or a
// CRLF hashes to the same value — which is the whole reason §3.3 says
// normalised rather than just hashed.
func TestTheClaimHashesAreTheTwoPreImagesSection33Names(t *testing.T) {
	const issue = "An expired token is rejected.\r\nThe endpoint returns 401.  \n"
	claims := []*Claim{
		{ID: "CR-1#c1", Text: "the token is checked", Span: "expired tokens are rejected"},
		{ID: "CR-1#c2", Text: "the status is 401", Span: "returns 401"},
	}
	require.NoError(t, ComputeClaimHashes(claims, issue))

	issueHash, err := text.NormalisedHash(issue)
	require.NoError(t, err)
	for _, claim := range claims {
		spanHash, err := text.NormalisedHash(claim.Span)
		require.NoError(t, err)
		assert.Equal(t, spanHash, claim.SpanHash, "%s hashes its own span", claim.ID)

		textHash, err := text.NormalisedHash(claim.Text)
		require.NoError(t, err)
		assert.NotEqual(t, textHash, claim.SpanHash,
			"§3.3 hashes the span the claim was drawn from, not the agent's wording of it")

		assert.Equal(t, issueHash, claim.IssueHash,
			"every claim of one extraction carries the same issue hash")
	}

	assert.NotEqual(t, claims[0].SpanHash, claims[1].SpanHash,
		"two spans of one issue are two hashes")

	whitespace := []*Claim{{ID: "CR-1#c3", Span: "expired  tokens\tare rejected  \r\n"}}
	require.NoError(t, ComputeClaimHashes(whitespace, issue))
	assert.Equal(t, claims[0].SpanHash, whitespace[0].SpanHash,
		"§1.4 collapses runs, strips trailing whitespace, and folds CRLF before the digest")
}

// §1.4 step 1 makes text that does not decode as UTF-8 fail with exit code 1,
// and a hash function that fell back to the raw bytes would turn that refusal
// into a value indistinguishable from every other sixteen-character hash.
// Neither pre-image is exempt, and each names what the user has to re-encode:
// the issue text is one text for the whole extraction, while a span belongs to
// one claim, so the refusal names that claim's id.
func TestTextThatDoesNotDecodeNeverBecomesAClaimHash(t *testing.T) {
	undecodable := "a" + string([]byte{0xFF}) + "b"

	claims := []*Claim{{ID: "CR-1#c1", Span: "a span that decodes"}}
	err := ComputeClaimHashes(claims, undecodable)
	var invalid *text.InvalidUTF8Error
	require.ErrorAs(t, err, &invalid)
	assert.Contains(t, err.Error(), "hashing the issue text")
	assert.Empty(t, claims[0].IssueHash, "nothing is stamped from a text cr could not read")
	assert.Empty(t, claims[0].SpanHash)

	spans := []*Claim{{ID: "CR-1#c4", Span: undecodable}}
	err = ComputeClaimHashes(spans, "an issue that decodes")
	require.ErrorAs(t, err, &invalid)
	assert.Contains(t, err.Error(), "hashing the span of CR-1#c4",
		"a span belongs to one claim, so the refusal names it")
	assert.Empty(t, spans[0].SpanHash)
}
