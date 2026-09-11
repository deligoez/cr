package cli

import (
	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/state"
)

// draftProvenances reads what §8.1.6's provenance region names from outside
// the queued records: the rationale of each rule a stored citation's
// `origin: rule` was matched against, per §2.6 item 4, and the note — with its
// §3.6.3 source — behind every claim of the round with `source: note`.
//
// Each half is read only when a queued record needs it. The corpus is resolved
// when some record carries a rule-origin citation, and the claims and the
// issue's notes when some record names a claim; a round with neither reads
// nothing more than it did before the region existed. The probes §8.1.7's
// evidence region carries are read the same way, when some record is graded
// `probed`.
func draftProvenances(
	l state.Layout, owner, repo string, pr int, round *state.Meta, queued []*finding.Finding,
) (*draft.Provenances, error) {
	sources := &draft.Provenances{
		Rationales: make(map[string]string),
		NoteClaims: make(map[string]draft.NoteClaim),
		Probes:     make(map[string]*probe.Record),
	}
	ruleCited, claimed, probed := false, false, false
	for _, record := range queued {
		claimed = claimed || record.Claim != ""
		probed = probed || record.Grade == finding.GradeProbed
		for at := range record.Citations {
			ruleCited = ruleCited || record.Citations[at].Origin == finding.OriginRule
		}
	}
	if ruleCited {
		corpus, err := roundCorpus(l, owner, repo, round.ProfileID)
		if err != nil {
			return nil, err
		}
		for i := range corpus {
			sources.Rationales[corpus[i].Rule.ID] = corpus[i].Rule.Rationale
		}
	}
	if claimed {
		if err := readNoteClaims(l, owner, repo, pr, round, sources.NoteClaims); err != nil {
			return nil, err
		}
	}
	if probed {
		if err := readProbes(l, owner, repo, pr, sources.Probes); err != nil {
			return nil, err
		}
	}
	return sources, nil
}

// readProbes fills into probes.ndjson's records by id, for §8.1.7's evidence
// region of every `probed` record.
//
// The whole file is read rather than the current round's lines: a probe is
// graded by id, §5.5.1 keeps every record the pull request ever wrote, and
// §5.5.3 has already decided at grading time which of them may stand behind
// a grade in this round. What the region carries is whatever that decision
// rested on.
func readProbes(l state.Layout, owner, repo string, pr int, into map[string]*probe.Record) error {
	stored, err := state.ReadRecords[probe.Record](l, owner, repo, pr, state.FileProbes)
	if err != nil {
		return err
	}
	for i := range stored {
		into[stored[i].ID] = &stored[i]
	}
	return nil
}

// readNoteClaims fills into the note behind every claim of the round drawn
// from the context store, with the source the store records for that note.
//
// The notes are the issue key's, as §3.6.4 loads them, and a retracted note is
// still named: §3.6.6 retracts by marking rather than erasing, precisely so a
// claim resting on it still points at something the author can look up. A
// claim whose note the store no longer holds keeps its note id and no source.
func readNoteClaims(
	l state.Layout, owner, repo string, pr int, round *state.Meta, into map[string]draft.NoteClaim,
) error {
	claims, err := state.ReadRecords[intent.Claim](l, owner, repo, pr, state.FileClaims)
	if err != nil {
		return err
	}
	sources := make(map[string]string)
	if round.IssueKey != "" {
		notes, err := note.Load(l, round.IssueKey)
		if err != nil {
			return err
		}
		for i := range notes {
			sources[notes[i].ID] = string(notes[i].Source)
		}
	}
	for i := range claims {
		claim := &claims[i]
		if claim.Round == round.Round && claim.Source == intent.ClaimFromNote {
			into[claim.ID] = draft.NoteClaim{Note: claim.NoteID, Source: sources[claim.NoteID]}
		}
	}
	return nil
}
