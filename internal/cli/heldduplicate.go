package cli

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// markHeldDuplicates is §6.4.5: a record `cr record` is about to store whose
// §6.4.1 identity equals that of a record the round already holds in `draft`
// or `queued` is stored as a duplicate of the held record.
//
// §9.3.4 is what makes this reachable. It carries a record into the round a
// push opens, and that round's fan-out reviews the same unit again, so a role
// that finds the same concern raises it a second time — and without this the
// draft would carry two comments on one line. `cr merge` deduplicates only the
// files it is handed, and the carried record is in none of them.
//
// The held record is kept as the representative rather than re-elected by
// §6.4.2. It is the one the reviewer may already have edited, and its body is
// the one §7.1.7 carried; a newly recorded twin that won on grade would take the
// comment away from the words the reviewer chose.
//
// A record already marked, by `cr merge`'s `duplicate_of` or by an agent's
// `suppressed_by`, is left as it is.
func markHeldDuplicates(l state.Layout, owner, repo string, pr, round int, records []*finding.Finding) error {
	stored, err := roundFindingsOf(l, owner, repo, pr, round)
	if err != nil {
		return err
	}
	held := make(map[finding.DedupKey]string)
	for _, record := range stored {
		if record.State == finding.StateDraft || record.State == finding.StateQueued {
			key := finding.DedupKeyOf(record)
			if _, taken := held[key]; !taken {
				held[key] = record.ID
			}
		}
	}
	for _, record := range records {
		if record.DuplicateOf != "" || record.SuppressedBy != "" {
			continue
		}
		if id, found := held[finding.DedupKeyOf(record)]; found {
			record.DuplicateOf = id
		}
	}
	return nil
}
