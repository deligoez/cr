package cli

import (
	"slices"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// ingestDraft is §7.1.6's first step: the triage state of the draft the
// round's queued records were rendered into, read through §7.2's verbs before
// that draft is replaced.
//
// A record in `queued` is one an earlier `cr draft` of this round rendered, so
// its block is either still in draft.md or the reviewer deleted it. A round
// holding no queued record has no draft of cr's to ingest — its draft.md is the
// empty file §2.3 starts a round with — and is read as a first rendering.
//
// Both discard verbs take effect at this moment, as §7.1.6 has it for a
// deletion: the record walks §9.1's `queued` → `discarded` row with `cr draft`
// as the actor, and takes `not-here` when its block was deleted or `wrong` when
// its marker says so. Its waiver is written by waiveDiscards once the new draft
// has rendered, so a run refused on the way leaves nothing behind. Nothing here
// re-renders a discarded record: queueRecords renders only `draft` and
// `queued`, so a discarded block is never resurrected.
//
// A softened finding is left a finding in findings.ndjson. The draft is where
// the reviewer said it, and it keeps saying it there: softenForDraft renders the
// block as a question, the marker then reads `kind=question` against a stored
// finding on every later run, and whoever interprets the draft next — this
// command again, or `cr post` — reads the same softening out of the same file.
// Stored, it would read as a question the agent wrote, and §7.3's `softened`
// would have nothing left to be counted from.
func ingestDraft(
	l state.Layout, owner, repo string, pr int, round *state.Meta, records []*finding.Finding,
) (triaged, error) {
	rendered := make([]*finding.Finding, 0, len(records))
	for _, record := range records {
		if record.State == finding.StateQueued {
			rendered = append(rendered, record)
		}
	}
	if len(rendered) == 0 {
		return triaged{Triage: draft.Triage{Preserved: make(map[string]string)}}, nil
	}
	file, err := l.ReadRound(owner, repo, pr, round.Round, state.FileDraft)
	if err != nil {
		return triaged{}, err
	}
	stored, err := l.ReadRound(owner, repo, pr, round.Round, state.FileRendered)
	if err != nil {
		return triaged{}, err
	}
	entries, err := draft.DecodeRendered(state.FileRendered, stored)
	if err != nil {
		return triaged{}, err
	}
	triage, err := draft.Ingest(rendered, &draft.Draft{
		Name:     l.RoundFile(owner, repo, pr, round.Round, state.FileDraft),
		Body:     string(file),
		Rendered: entries,
		Trees:    anchorTrees(owner, repo, pr, round.Head),
	})
	if err != nil {
		return triaged{}, err
	}
	// A slice rather than a map, so a refusal names the same record on
	// every run, as §2.1.1 asks of every command.
	discards := []struct {
		disposition finding.Disposition
		records     []*finding.Finding
	}{
		{finding.DispositionNotHere, triage.Deleted},
		{finding.DispositionWrong, triage.Wrong},
	}
	for _, verb := range discards {
		disposition := verb.disposition
		for _, record := range verb.records {
			if err := finding.MayTransition(
				record.ID, finding.Existing(finding.StateQueued),
				finding.StateDiscarded, finding.ActorDraft,
			); err != nil {
				return triaged{}, err
			}
			record.State, record.Disposition = finding.StateDiscarded, disposition
		}
	}
	applyRetriage(triage.Retriaged)
	return triaged{Triage: triage, read: rendered}, nil
}

// anchorTrees are §6.1.2's two revisions for this round, each opened only when
// something actually reads it.
//
// §7.2 re-validates a marker's `path`, `start_line` and `line` against them,
// and that is the only thing in `cr draft` which reads the repository at all.
// So a run where no marker moved an anchor must not require a checkout, and one
// that moved a RIGHT anchor must not require the pull request — both closures
// resolve what they need on first use and not before, which keeps the ordinary
// run, where the reviewer edited prose, exactly as cheap as it was.
//
// The merge base is taken the way §3.4.1 takes it, against the base GitHub
// reports for the pull request, so a LEFT anchor is re-validated against the
// same revision this round's units were formed against.
func anchorTrees(owner, repo string, pr int, head string) finding.Trees {
	var dir, base string
	checkout := func() (string, error) {
		if dir == "" {
			resolved, err := repoDir()
			if err != nil {
				return "", err
			}
			dir = resolved
		}
		return dir, nil
	}
	return finding.Trees{
		Head: func(path string) ([]string, bool, error) {
			where, err := checkout()
			if err != nil {
				return nil, false, err
			}
			return git.FileAtRevision(where, head, path)
		},
		MergeBase: func(path string) ([]string, bool, error) {
			where, err := checkout()
			if err != nil {
				return nil, false, err
			}
			if base == "" {
				opened, err := ghClient().PullRequest(owner, repo, pr)
				if err != nil {
					return nil, false, err
				}
				merged, err := git.MergeBase(where, opened.Base, head)
				if err != nil {
					return nil, false, err
				}
				base = merged
			}
			return git.FileAtRevision(where, base, path)
		},
	}
}

// applyRetriage writes the admitted marker edits onto the records they were
// read against: §7.2's `severity` row, and the re-validated anchor of its
// location row, carrying §9.2's content hash recomputed over the lines it now
// names.
//
// These reach findings.ndjson where a softening does not, and the asymmetry is
// §7.2's own. A softening is a verb the draft goes on saying, so storing it
// would leave §7.3's `softened` nothing to count; these two are values cr has
// accepted, and a value left unstored would be overwritten by the marker the
// next rendering writes back out of the record.
func applyRetriage(edits []draft.Retriage) {
	for _, edit := range edits {
		if edit.Severity != "" {
			edit.Record.Severity = edit.Severity
		}
		if edit.Anchor != nil {
			edit.Record.Anchor = *edit.Anchor
		}
	}
}

// triaged is what ingestDraft read: the triage, and the records it was read
// over, in the order they arrived.
type triaged struct {
	draft.Triage
	read []*finding.Finding
}

// discarded are the records this run's triage discards, whatever the verb.
func (t *triaged) discarded() []*finding.Finding {
	return append(append(make([]*finding.Finding, 0, len(t.Deleted)+len(t.Wrong)), t.Deleted...), t.Wrong...)
}

// triagedRecord is one record §7.2's verbs moved away from `kept`, as
// `cr draft` reports it: the outcome §7.3.1 names, and whether §7.3.4 counts it
// against the record's class.
type triagedRecord struct {
	// ID is the record's id.
	ID string `json:"id"`
	// Outcome is §7.3.1's outcome action.
	Outcome finding.Outcome `json:"outcome"`
	// CountsAgainstClass is whether the outcome enters §7.3.4's demotion
	// numerator, which a `wrong` does and a deletion never does.
	CountsAgainstClass bool `json:"counts_against_class"`
}

// report lists every record the triage did not keep, in the order the records
// arrived, never nil.
func (t *triaged) report() []triagedRecord {
	out := make([]triagedRecord, 0, len(t.Deleted)+len(t.Wrong)+len(t.Softened))
	for _, record := range t.read {
		outcome := t.Outcome(record)
		if outcome != finding.OutcomeKept {
			out = append(out, triagedRecord{
				ID: record.ID, Outcome: outcome, CountsAgainstClass: outcome.CountsAgainstClass(),
			})
		}
	}
	return out
}

// softenForDraft renders each softened finding as a question, without
// touching the stored record: the queued slice gets a copy whose kind is the
// one the reviewer's marker asked for.
func softenForDraft(queued, softened []*finding.Finding) []*finding.Finding {
	out := make([]*finding.Finding, 0, len(queued))
	for _, record := range queued {
		if slices.Contains(softened, record) {
			asked := *record
			asked.Kind = finding.KindQuestion
			record = &asked
		}
		out = append(out, record)
	}
	return out
}

// waiveDiscards writes the waiver each discard of this run calls for, per
// §7.2 and §7.4: a deletion's `not-here` scopes it to this pull request and a
// `wrong` to the repository, and finding.Waive derives the scope from the
// disposition rather than taking it from here.
//
// The waivers are written before the records that name them as discarded, so a
// run interrupted between the two leaves a record still queued behind a waiver
// rather than a discard with no waiver. The next run reads the same draft,
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
