package cli

import (
	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// ingestDraft is §7.1.6's first step: the triage state of the draft the
// round's queued records were rendered into, read before that draft is
// replaced.
//
// A record in `queued` is one an earlier `cr draft` of this round rendered, so
// its block is either still in draft.md or the reviewer deleted it. A round
// holding no queued record has no draft of cr's to ingest — its draft.md is the
// empty file §2.3 starts a round with — and is read as a first rendering.
//
// Each deleted block becomes a discard at this moment, as §7.1.6 has it: the
// record walks §9.1's `queued` → `discarded` row with `cr draft` as the actor
// and takes the `not-here` disposition §7.2 gives a deletion. Its waiver is
// written by waiveDiscards once the new draft has rendered, so a run refused
// on the way leaves nothing behind. Nothing here re-renders a discarded record:
// queueRecords renders only `draft` and `queued`, so a deleted block is never
// resurrected.
func ingestDraft(
	l state.Layout, owner, repo string, pr int, round *state.Meta, records []*finding.Finding,
) (draft.Triage, error) {
	rendered := make([]*finding.Finding, 0, len(records))
	for _, record := range records {
		if record.State == finding.StateQueued {
			rendered = append(rendered, record)
		}
	}
	if len(rendered) == 0 {
		return draft.Triage{Deleted: make([]*finding.Finding, 0), Preserved: make(map[string]string)}, nil
	}
	file, err := l.ReadRound(owner, repo, pr, round.Round, state.FileDraft)
	if err != nil {
		return draft.Triage{}, err
	}
	stored, err := l.ReadRound(owner, repo, pr, round.Round, state.FileRendered)
	if err != nil {
		return draft.Triage{}, err
	}
	entries, err := draft.DecodeRendered(state.FileRendered, stored)
	if err != nil {
		return draft.Triage{}, err
	}
	triage, err := draft.Ingest(rendered, string(file), entries)
	if err != nil {
		return draft.Triage{}, err
	}
	for _, record := range triage.Deleted {
		if err := finding.MayTransition(
			record.ID, finding.Existing(finding.StateQueued),
			finding.StateDiscarded, finding.ActorDraft,
		); err != nil {
			return draft.Triage{}, err
		}
		record.State, record.Disposition = finding.StateDiscarded, finding.DispositionNotHere
	}
	return triage, nil
}

// waiveDiscards writes the waiver each discard of this run calls for, per
// §7.2 and §7.4: a deletion's `not-here` scopes it to this pull request, and
// finding.Waive derives that from the disposition rather than taking it from
// here.
//
// The waivers are written before the records that name them as discarded, so a
// run interrupted between the two leaves a record still queued behind a waiver
// rather than a discard with no waiver. The next run reads the same deletion,
// discards the record again, and finding.Waive returns the waiver already
// there instead of writing a second.
func waiveDiscards(l state.Layout, owner, repo string, pr int, discarded []*finding.Finding) error {
	for _, record := range discarded {
		waiver, err := finding.WaiverFor(record)
		if err != nil {
			return err
		}
		if _, err := finding.Waive(l, owner, repo, &waiver, finding.WaiverProvenance{
			Round: record.Round, PR: pr, Head: record.Head,
		}); err != nil {
			return err
		}
	}
	return nil
}

// recordIDs are the ids of records, in their order, never nil.
func recordIDs(records []*finding.Finding) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids
}

// preservedIDs are the ids of the queued records whose body was preserved, in
// the order the draft holds them.
func preservedIDs(queued []*finding.Finding, preserved map[string]string) []string {
	ids := make([]string, 0, len(preserved))
	for _, record := range queued {
		if _, kept := preserved[record.ID]; kept {
			ids = append(ids, record.ID)
		}
	}
	return ids
}
