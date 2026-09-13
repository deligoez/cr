package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// refuseHeldIDs refuses a file whose record ids repeat within it, or collide
// with a record already stored for the pull request, naming the id and both
// lines or the line and the stored record, and the next free id.
//
// §6.1 makes a record id stable for the life of the pull request, and every
// store that names a record — a draft marker, a waiver, the posted index, a
// triage event — names it by that id alone. Two records holding one id would
// let any of them reach either.
//
// It lives at `cr record`'s acceptance rather than inside finding.Decode for
// the reason the second half needs: the stored ids are the pull request's
// state, and the decoder is handed a file and the round's units, never the
// store. `cr record` is the only command that adds a record to
// findings.ndjson, so no route to the store passes around it, and the repeat
// within one file is checked beside the collision so the one refusal answers
// both. The store is read whole, across rounds, because an id an earlier
// round's record holds is held still: §9.3.4 moves such a record to `stale`
// and keeps it.
//
// The next free id is finding.NextID over the store and the whole file, so the
// id a refusal suggests is held neither by an earlier round nor by a later line
// of the same file.
func refuseHeldIDs(
	l state.Layout, owner, repo string, pr int, file string, body []byte, records []*finding.Finding,
) error {
	stored, err := state.ReadRecords[finding.Finding](l, owner, repo, pr, state.FileFindings)
	if err != nil {
		return err
	}
	held := make(map[string]*finding.Finding, len(stored))
	for i := range stored {
		held[stored[i].ID] = &stored[i]
	}
	nextFree := func() string {
		taken := make([]finding.Finding, 0, len(stored)+len(records))
		taken = append(taken, stored...)
		for _, record := range records {
			taken = append(taken, *record)
		}
		return finding.NextID(taken)
	}
	at := state.RecordLines(body)
	seen := make(map[string]int, len(records))
	for i, record := range records {
		if first, repeated := seen[record.ID]; repeated {
			return &finding.RejectedRecordError{
				File: file, Line: at[i], Field: "id",
				Problem: fmt.Sprintf(
					"%q repeats the id of line %d; §6.1 makes a record id stable for the life of the pull request, "+
						"so give each record an id of its own; the next free id is %s", record.ID, first, nextFree()),
			}
		}
		seen[record.ID] = at[i]
		if prior, taken := held[record.ID]; taken {
			return &finding.RejectedRecordError{
				File: file, Line: at[i], Field: "id",
				Problem: fmt.Sprintf(
					"%q is already held by the record stored for this pull request in round %d at head %s; "+
						"§6.1 makes a record id stable for the life of the pull request, so give this record an id no stored record holds; "+
						"the next free id is %s",
					record.ID, prior.Round, prior.Head, nextFree()),
			}
		}
	}
	return nil
}
