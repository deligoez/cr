package cli

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// recordDrops is what `cr record` took back out of its input before anything
// was written: §6.4.4's waived findings and §9.3.6's already-posted ones.
type recordDrops struct {
	waived finding.Drops
	posted finding.PostedDrops
}

// settleAnchors binds every record's anchor to its own unit and then stamps
// §9.2.3's content hash and context window on it. The two are one step because
// the second is what §6.4.4 and §9.3.6 match on: a record reaches the drops
// below carrying the hash of its own lines at the round's head, never the one
// the agent typed.
func settleAnchors(
	owner, repo string, pr int, round *state.Meta, file string, body []byte,
	formed []roundUnit, records []*finding.Finding,
) error {
	if err := refuseForeignAnchors(owner, repo, pr, round, file, body, formed, records); err != nil {
		return err
	}
	return stampAnchors(owner, repo, pr, round, file, body, records)
}

// dropRecorded re-applies §6.4.4's waiver drops and §9.3.6's posted-index drops
// to the records `cr record` accepted, and then walks §9.1's first two rows over
// the records that remain.
//
// `cr merge` applies both drops already, and §6.5.1 lists them among its passes.
// They are applied again here because nothing requires `cr record`'s input to be
// `cr merge`'s output — §11 lists `cr record <pr> <file>` with no precondition,
// and a single role's file is the obvious thing to hand it — and the invariant
// is that a waived or already-posted finding never reaches findings.ndjson, not
// that a particular command ran. Over `cr merge`'s own output the second pass
// matches nothing, so it is idempotent.
//
// The states are walked after the drops rather than before them, so a dropped
// record is never brought into `draft` and no §9.1.1 journal entry is written
// for a record findings.ndjson will not hold. The drops run after every refusal
// that names an input line, because those refusals index the records by their
// position in the file.
func dropRecorded(
	l state.Layout, owner, repo string, pr int, records []*finding.Finding, journal *finding.Journal,
) ([]*finding.Finding, recordDrops, error) {
	waivers, err := finding.ActiveWaivers(l, owner, repo, pr)
	if err != nil {
		return nil, recordDrops{}, err
	}
	kept, waived := finding.DropWaived(records, waivers)
	index, err := finding.PostedIndex(l, owner, repo, pr)
	if err != nil {
		return nil, recordDrops{}, err
	}
	kept, posted := finding.DropPosted(kept, index)
	if err := stampStates(kept, journal); err != nil {
		return nil, recordDrops{}, err
	}
	return kept, recordDrops{waived: waived, posted: posted}, nil
}
