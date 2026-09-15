package intent

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/text"
)

// legacyIssue is issue text as `jira issue view --plain` printed it on the
// field trial: a U+00A0 inside a requirement, and a footer in colour.
const legacyIssue = "CR-1: an order over 50 TL ships free.\n\nAn order under 50 TL pays shipping.\n" +
	"\x1b[38;5;242mView this issue on Jira\x1b[0m\n"

// legacyClaim is a claim as a release before cleaning recorded it: its span
// copied with the U+00A0, and its issue_hash taken over the bytes as printed.
func legacyClaim(t *testing.T) Claim {
	t.Helper()
	hash, err := text.NormalisedHash(legacyIssue)
	require.NoError(t, err)
	return Claim{ID: "CR-1#c1", Source: ClaimFromDescription, Span: "over 50 TL ships free", IssueHash: hash}
}

// Field feedback 1.2's verification: cleaning an existing round's issue text
// moved its hash and took away a span holding a U+00A0. A claim recorded before
// cleaning, over an issue that has not changed, reports no drift and a span
// that still occurs.
func TestAClaimRecordedBeforeCleaningReportsNoFalseDrift(t *testing.T) {
	claim := legacyClaim(t)
	reading := read(legacyIssue)
	require.NotEqual(t, legacyIssue, reading.Text, "the control: cleaning changed this text")

	drift, err := DetectDrift(slices.Values([]Claim{claim}), reading.Spans(nil))
	require.NoError(t, err)

	cleaned, err := text.NormalisedHash(reading.Text)
	require.NoError(t, err)
	assert.Equal(t, Drift{Hash: cleaned, Claims: []ClaimDrift{
		{ID: "CR-1#c1", ExtractedHash: claim.IssueHash, SpanOccurs: true},
	}}, drift)
}

// The other direction: the same claim over an issue that did change still
// drifts, and a span the changed text no longer holds, in either form, is gone.
func TestAClaimRecordedBeforeCleaningStillDriftsWhenTheIssueMoves(t *testing.T) {
	claim := legacyClaim(t)
	moved := "CR-1: an order over 75 TL ships free.\n\nAn order under 50 TL pays shipping.\n" +
		"\x1b[38;5;242mView this issue on Jira\x1b[0m\n"

	drift, err := DetectDrift(slices.Values([]Claim{claim}), read(moved).Spans(nil))
	require.NoError(t, err)

	assert.True(t, drift.Drifted)
	assert.Equal(t, []ClaimDrift{{ID: "CR-1#c1", ExtractedHash: claim.IssueHash, SpanOccurs: false}}, drift.Claims)
}

// A claim recorded since, over the cleaned text, answers to the cleaned hash:
// an unchanged issue does not drift, and a colour change in the footer alone,
// which cleaning removes, does not either.
func TestAClaimRecordedOverTheCleanedTextAnswersToTheCleanedHash(t *testing.T) {
	reading := read(legacyIssue)
	claims := []*Claim{{ID: "CR-1#c1", Source: ClaimFromDescription, Span: "over 50 TL ships free"}}
	require.NoError(t, ComputeClaimHashes(claims, reading.Text))

	recolored := "CR-1: an order over 50 TL ships free.\n\nAn order under 50 TL pays shipping.\n" +
		"\x1b[1mView this issue on Jira\x1b[0m\n"
	for name, issue := range map[string]string{"unchanged": legacyIssue, "recoloured": recolored} {
		t.Run(name, func(t *testing.T) {
			drift, err := DetectDrift(slices.Values([]Claim{*claims[0]}), read(issue).Spans(nil))
			require.NoError(t, err)
			assert.Equal(t, Drift{Hash: claims[0].IssueHash, Claims: []ClaimDrift{
				{ID: "CR-1#c1", ExtractedHash: claims[0].IssueHash, SpanOccurs: true},
			}}, drift)
		})
	}
}

// A span recorded before cleaning is asked both ways, and each way is needed:
// one holding a U+00A0 still occurs after the tracker stops printing it, where
// only its cleaned form can be found, and one reaching into an escape sequence
// still occurs in the bytes as printed, where only its raw form can. The hash
// moved in the first case — the bytes did — so the claim drifts, and its span
// is reported present.
func TestASpanRecordedBeforeCleaningIsFoundInEitherForm(t *testing.T) {
	claim := legacyClaim(t)
	spaced := Clean(legacyIssue)
	drift, err := DetectDrift(slices.Values([]Claim{claim}), read(spaced).Spans(nil))
	require.NoError(t, err)
	assert.True(t, drift.Drifted)
	assert.Equal(t, []ClaimDrift{{ID: "CR-1#c1", ExtractedHash: claim.IssueHash, SpanOccurs: true}}, drift.Claims)

	claim.Span = "242mView this issue"
	drift, err = DetectDrift(slices.Values([]Claim{claim}), read(legacyIssue).Spans(nil))
	require.NoError(t, err)
	assert.Equal(t, Drift{Hash: drift.Hash, Claims: []ClaimDrift{
		{ID: "CR-1#c1", ExtractedHash: claim.IssueHash, SpanOccurs: true},
	}}, drift)
}

// §3.3.1 is unchanged by cleaning: a span is checked verbatim against the
// cleaned text, so the span as `cr brief` prints it is accepted and one still
// carrying the U+00A0 the text no longer has is refused.
func TestAClaimSpanIsCheckedVerbatimAgainstTheCleanedText(t *testing.T) {
	spans := read(legacyIssue).Spans(nil)
	checker := claimChecker{file: "claims.ndjson", issueKey: "CR-1", spans: spans}

	printed := Claim{ID: "CR-1#c1", Source: ClaimFromDescription, Span: "over 50 TL ships free"}
	require.NoError(t, checker.span(1, &printed))

	copied := Claim{ID: "CR-1#c1", Source: ClaimFromDescription, Span: "over 50 TL ships free"}
	var rejected *RejectedClaimError
	require.ErrorAs(t, checker.span(1, &copied), &rejected)
	assert.Equal(t, "span", rejected.Field)
}
