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
// from the context store whose note still stands, with the source the store
// records for that note.
//
// The notes are the issue key's, as §3.6.4 loads them. A claim whose note was
// retracted, or whose note the store no longer holds, is left out, so §8.1.6's
// region never names a note id that no longer resolves to a standing note:
// note.Standing.Stands is the one predicate both the region and the register
// read, and a record resting on such a claim is held as a question by
// withdrawnClaims' caller and reported under §3.6.6 instead.
func readNoteClaims(
	l state.Layout, owner, repo string, pr int, round *state.Meta, into map[string]draft.NoteClaim,
) error {
	claims, notes, err := roundNoteClaims(l, owner, repo, pr, round)
	if err != nil {
		return err
	}
	for i := range claims {
		held, found := note.Find(notes, claims[i].NoteID)
		if found && held.Standing().Stands() {
			into[claims[i].ID] = draft.NoteClaim{Note: held.ID, Source: string(held.Source)}
		}
	}
	return nil
}

// withdrawnClaims is §3.6.6 over the round's claims: the id of every claim drawn
// from the context store whose note no longer stands, retracted or missing
// alike, as note.StandingOf reports it now.
//
// It is read at the moment `cr draft` or `cr post` runs rather than stamped at
// `cr record`, so a note retracted mid-round takes the assertion register away
// from the records resting on it in the same round.
func withdrawnClaims(l state.Layout, owner, repo string, pr int, round *state.Meta) (map[string]bool, error) {
	claims, notes, err := roundNoteClaims(l, owner, repo, pr, round)
	if err != nil {
		return nil, err
	}
	withdrawn := make(map[string]bool, len(claims))
	for i := range claims {
		if !note.StandingOf(notes, claims[i].NoteID).Stands() {
			withdrawn[claims[i].ID] = true
		}
	}
	return withdrawn, nil
}

// roundNoteClaims reads the round's claims with `source: note` and every note
// the issue key's store holds, which is what note.StandingOf needs whole.
//
// A round that resolved no issue key has no store to load, so every
// note-sourced claim in it reads as resting on a note the store does not hold.
func roundNoteClaims(
	l state.Layout, owner, repo string, pr int, round *state.Meta,
) ([]intent.Claim, []note.Note, error) {
	claims, err := state.ReadStamped[intent.Claim](
		l, owner, repo, pr, state.FileClaims, round.Round)
	if err != nil {
		return nil, nil, err
	}
	var notes []note.Note
	if round.IssueKey != "" {
		notes, err = note.Load(l, round.IssueKey)
		if err != nil {
			return nil, nil, err
		}
	}
	drawn := make([]intent.Claim, 0, len(claims))
	for i := range claims {
		if claims[i].Source == intent.ClaimFromNote {
			drawn = append(drawn, claims[i])
		}
	}
	return drawn, notes, nil
}
