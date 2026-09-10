package rule

import "github.com/deligoez/cr/internal/finding"

// StampOrigins computes §6.2.5's `origin` onto every citation of every record,
// against cr's own detection output as the repository's ledger holds it.
//
// A citation gets `origin: rule` when the ledger holds a hit for the same head,
// the same rule id as the record names, and the same path and line; every other
// citation gets `origin: agent`. The match is positional on purpose. Matching
// on the rule id alone would let a record name any rule and point anywhere
// inside its own unit, and §6.2's `cited` row would then grade it as though a
// detector had found it — which is exactly the forgery the deleted `rule` grade
// branch allowed. The rule id is the record's own, per §2.6.1.3, so a citation
// is judged against the rule the record says produced it and against no other.
//
// Only a hit counts. A record event or a dismissal at the same place is cr's
// account of what the agent did, not of what a detector found, so it can
// support no citation.
//
// The value is overwritten rather than merged. §6.1.4 has already refused a
// record arriving with any `origin` at all, so what is written here is the
// field's only author.
func StampOrigins(ledger []Stat, head string, records []*finding.Finding) {
	for _, record := range records {
		for at := range record.Citations {
			citation := &record.Citations[at]
			citation.Origin = finding.OriginAgent
			if record.Rule != "" && detected(ledger, head, record.Rule, citation) {
				citation.Origin = finding.OriginRule
			}
		}
	}
}

// detected reports whether the ledger holds a hit of rule at the citation's
// path and line for head.
func detected(ledger []Stat, head, rule string, citation *finding.Citation) bool {
	for at := range ledger {
		hit := &ledger[at]
		if hit.Event == EventHit && hit.Head == head && hit.Rule == rule &&
			hit.Path == citation.Path && hit.Line == citation.Line {
			return true
		}
	}
	return false
}
