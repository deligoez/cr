package cli

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/role"
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
// to the records `cr record` accepted, and hands back, beside the records that
// remain, the ones a drop orphaned.
//
// `cr merge` applies both drops already, and §6.5.1 lists them among its passes.
// They are applied again here because nothing requires `cr record`'s input to be
// `cr merge`'s output — §11 lists `cr record <pr> <file>` with no precondition,
// and a single role's file is the obvious thing to hand it — and the invariant
// is that a waived or already-posted finding never reaches findings.ndjson, not
// that a particular command ran. Over `cr merge`'s own output the second pass
// matches nothing, so it is idempotent.
//
// A drop can take out a duplicate group's representative when a waiver appeared
// between `cr merge` and `cr record`. The group's other records would then keep
// a `duplicate_of` naming a record the round never holds, and `duplicate` is
// terminal, so a finding the waiver did not cover would vanish unreported. Such
// a record has its `duplicate_of` cleared and is returned as orphaned, for
// settleStates to re-elect §6.4.2's representative among once grades exist. A
// `duplicate_of` naming a record this round already stored is not orphaned.
//
// The drops run after every refusal that names an input line, because those
// refusals index the records by their position in the file.
func dropRecorded(
	l state.Layout, owner, repo string, pr, round int, records []*finding.Finding,
) (kept, orphans []*finding.Finding, dropped recordDrops, err error) {
	waivers, err := finding.ActiveWaivers(l, owner, repo, pr)
	if err != nil {
		return nil, nil, recordDrops{}, err
	}
	kept, waived := finding.DropWaived(records, waivers)
	index, err := finding.PostedIndex(l, owner, repo, pr)
	if err != nil {
		return nil, nil, recordDrops{}, err
	}
	kept, posted := finding.DropPosted(kept, index)
	stored, err := state.ReadStamped[finding.Finding](l, owner, repo, pr, state.FileFindings, round)
	if err != nil {
		return nil, nil, recordDrops{}, err
	}
	return kept, orphaned(kept, stored), recordDrops{waived: waived, posted: posted}, nil
}

// orphaned clears the `duplicate_of` of every kept record whose representative
// is neither kept nor already stored in the round, and returns those records.
func orphaned(kept []*finding.Finding, stored []finding.Finding) []*finding.Finding {
	present := make(map[string]bool, len(kept)+len(stored))
	for _, record := range kept {
		present[record.ID] = true
	}
	for i := range stored {
		present[stored[i].ID] = true
	}
	orphans := make([]*finding.Finding, 0)
	for _, record := range kept {
		if record.DuplicateOf != "" && !present[record.DuplicateOf] {
			record.DuplicateOf = ""
			orphans = append(orphans, record)
		}
	}
	return orphans
}

// settleStates grades the records, applies §4.1.4, re-elects §6.4.2's
// representative among every group dropRecorded orphaned, and then walks §9.1's
// first two rows over the records findings.ndjson will hold.
//
// The re-election needs the grades, because §6.4.2 ranks by grade first, and
// the states need the re-election, because `duplicate` is read off
// `duplicate_of`. Walking the states last also means a dropped record is never
// brought into `draft` and no §9.1.1 journal entry is written for a record
// findings.ndjson will not hold. A group whose every member was dropped has no
// orphan left to re-elect, and nothing of it is stored.
func settleStates(
	l state.Layout, owner, repo string, round *state.Meta, formed []roundUnit, evidence *roundEvidence,
	orphans, records []*finding.Finding, journal *finding.Journal,
) error {
	gradeRecords(round, formed, evidence, records)
	// §4.1.4: an intent-axis record on a unit the round's mapping maps to no
	// claim is a question, never a finding. It reads the axis stamped earlier
	// and moves only toward the question register.
	forceUnmappedIntent(round.Round, formed, evidence.pairs, records)
	if len(orphans) > 0 {
		corpus, err := role.Resolve(l.RepoRolesDir(owner, repo), l.RolesDir())
		if err != nil {
			return err
		}
		finding.MarkDuplicates(orphans, role.Order(corpus))
	}
	return stampStates(records, journal)
}
