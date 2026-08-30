package intent

import (
	"fmt"

	"github.com/deligoez/cr/internal/text"
)

// ComputeClaimHashes writes §3.3's two computed rows onto every claim of one
// extraction, which is what §3.3.1 means by "`cr` MUST compute `span_hash` and
// `issue_hash` itself".
//
// Neither pre-image is chosen here; §3.3's table states both. `span_hash` is
// the "Normalised hash of `span`", so each claim's own span and nothing around
// it. `issue_hash` is the "Normalised hash of the whole issue text at
// extraction", so the issue text as a whole — not the spans drawn from it, not
// the claims, and not the text of any note.
//
// The issue hash is computed once and shared, because §3.3 makes it one value
// per extraction rather than one per claim: §3.3.3 detects drift by re-reading
// the issue at the start of every round and comparing its normalised hash to
// "the stored `issue_hash`", singular. Two claims of one extraction that
// disagreed about it would make that comparison a question with two answers.
//
// A claim sourced from a note is no exception to either. §3.3.2 sets its span
// to the note's body, so its `span_hash` is that body's hash by the same rule,
// and its `issue_hash` is still the issue's — a claim carrying some other
// text's hash there would report drift in a round where the issue never moved.
//
// Both route through text.NormalisedHash, which runs §1.4's six steps on its
// input itself, so no unnormalised text can reach a digest by way of this
// function. Its error is §1.4 step 1's and is returned rather than swallowed:
// text that does not decode as UTF-8 fails with exit code 1, and a hash
// function that fell back to the raw bytes would turn that refusal into a value
// indistinguishable from every other one.
func ComputeClaimHashes(claims []*Claim, issueText string) error {
	issueHash, err := text.NormalisedHash(issueText)
	if err != nil {
		return fmt.Errorf("hashing the issue text: %w", err)
	}
	for _, claim := range claims {
		spanHash, err := text.NormalisedHash(claim.Span)
		if err != nil {
			return fmt.Errorf("hashing the span of %s: %w", claim.ID, err)
		}
		claim.SpanHash = spanHash
		claim.IssueHash = issueHash
	}
	return nil
}
