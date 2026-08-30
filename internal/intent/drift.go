package intent

import (
	"fmt"
	"iter"

	"github.com/deligoez/cr/internal/text"
)

// ClaimDrift is §3.3.3's answer about one claim: whether the span it was drawn
// from is still there.
//
// It carries an id, a hash and a boolean, and no claim. That is the shape the
// section's last sentence asks for — "`cr` MUST NOT re-extract; re-extraction
// happens only through a new `cr claims record`" — expressed as a type rather
// than as a rule to remember. There is no span here, no text, and no Claim, so
// there is nothing in a drift report a caller could store as a claim, and no
// path by which a changed issue text could turn into a changed claim.
type ClaimDrift struct {
	// ID is the claim's §3.3 id.
	ID string `json:"id"`
	// ExtractedHash is the `issue_hash` stored on the claim: the
	// normalised hash of the issue text as it read when the claim was
	// extracted.
	ExtractedHash string `json:"extracted_hash"`
	// SpanOccurs reports whether the claim's `span` still occurs in the
	// issue text as it now reads. It is asked of every claim rather than
	// only of the drifted ones, and it can be false while the hashes agree:
	// §1.4 normalises before hashing, and §3.3 calls a span the verbatim
	// substring of its source, so a change to whitespace alone leaves the
	// hash equal and can still take a span away.
	SpanOccurs bool `json:"span_occurs"`
}

// Drifted reports whether the issue text has moved since this claim was
// extracted. It is derived rather than stored, so a report cannot say one
// thing here and another in the two hashes it was derived from.
func (d ClaimDrift) Drifted(current string) bool {
	return d.ExtractedHash != current
}

// Drift is §3.3.3's comparison for one round: the issue text as it now reads,
// measured against the claims already recorded.
type Drift struct {
	// Hash is the normalised hash of the issue text at the moment the
	// comparison ran, per §1.4.
	Hash string `json:"hash"`
	// Drifted reports whether any claim was extracted against a different
	// issue text. §3.3.3 makes this the condition on the per-claim report,
	// and it is what `cr brief` prints per §3.7.3.
	Drifted bool `json:"drifted"`
	// Claims is one entry per claim, in the order they were given, which is
	// the order claims.ndjson holds them in. Nothing here is sorted or
	// deduplicated: the report is about the claims that exist, and a claim
	// missing from it would be a claim nobody was told about.
	Claims []ClaimDrift `json:"claims"`
}

// DetectDrift carries out §3.3.3: it hashes the issue text as it now reads,
// compares that against the `issue_hash` each claim was stored with, and
// reports for every claim whether its span still occurs.
//
// It reads and reports, and it is built so that it cannot do anything else.
// Three things hold at once, and each is a fact about the signature rather
// than a promise in a comment:
//
//   - It takes no *state.Lock. §2.3.1 admits no unlocked write to per-PR
//     state and state.Lock is the only route to those files, so this function
//     cannot write claims.ndjson, or any other file of §2.3, at all.
//   - It takes an iter.Seq[Claim] rather than a slice. The sequence yields
//     copies, so there is no backing array here to reach: a claim cannot be
//     altered in the caller's own hands either.
//   - It returns ClaimDrift and not Claim. A report holds an id, a hash and a
//     boolean, so there is nothing in the result that a caller could mistake
//     for an extraction and store.
//
// Together those are what "cr MUST NOT re-extract" means in code. Re-extraction
// happens only through a new `cr claims record`, which is a different function
// in a different file that takes the lock and is handed a file by an agent.
//
// The comparison is per claim rather than against one stored hash chosen from
// among them. ComputeClaimHashes writes one issue_hash across an extraction, so
// in a well-formed round every claim answers the same, and the two readings
// agree; where they do not, this one reports what each claim actually says
// instead of picking a winner and calling the rest agreed.
//
// The occurrence test is SpanOccursIn, the same predicate §3.3.1 rejects a
// claim by. Two readings of "occurs" could disagree, and the disagreement would
// be silent in the worst direction: a claim recorded under one reading and
// reported stale under the other.
func DetectDrift(claims iter.Seq[Claim], issueText string) (Drift, error) {
	current, err := text.NormalisedHash(issueText)
	if err != nil {
		return Drift{}, fmt.Errorf("hashing the issue text: %w", err)
	}
	drift := Drift{Hash: current, Claims: make([]ClaimDrift, 0)}
	for claim := range claims {
		reported := ClaimDrift{
			ID:            claim.ID,
			ExtractedHash: claim.IssueHash,
			SpanOccurs:    SpanOccursIn(issueText, claim.Span),
		}
		drift.Drifted = drift.Drifted || reported.Drifted(current)
		drift.Claims = append(drift.Claims, reported)
	}
	return drift, nil
}
