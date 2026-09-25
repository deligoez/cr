package cli

import (
	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/render"
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
		FileClaims: make(map[string]string),
		Probes:     make(map[string]*probe.Record),
		Head:       round.Head,
	}
	ruleCited, claimed, probed := false, false, false
	for _, record := range queued {
		claimed = claimed || record.Claim != ""
		// §8.1.7 carries a probe's re-runs beneath every record naming
		// it, whatever its grade, so any probe named is read.
		probed = probed || record.Grade == finding.GradeProbed || record.Probe != ""
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
		if err := readClaimProvenance(l, owner, repo, pr, round, sources); err != nil {
			return nil, err
		}
	}
	if probed {
		if err := readProbes(l, owner, repo, pr, sources.Probes); err != nil {
			return nil, err
		}
		// §8.1.7 shows a run in which nothing failed by the lines the
		// round's profile's tests.count_pattern matches.
		resolved, err := statusProfile(l, round.ProfileID)
		if err != nil {
			return nil, err
		}
		sources.CountPattern = resolved.Tests.CountPattern
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

// readClaimProvenance fills in what §8.1.6 names for the two claim sources it
// discloses: the note behind every claim of the round drawn from the context
// store, with the source the store records for that note and whether it still
// stands, and the extra intent file of §3.1.5 behind every claim drawn from
// one.
//
// The notes are the issue key's, as §3.6.4 loads them. §8.1.6 names the note
// behind every such claim with no exception for a withdrawn one, so a claim
// whose note was retracted keeps its note and source marked retracted, and a
// claim whose note the store no longer holds keeps its note id marked missing,
// with no source to name. The register is withdrawnClaims' business: a record
// resting on either is held as a question and reported under §3.6.6, and the
// region beside it says why.
//
// A file-sourced claim carries its path and nothing more. The document is not
// cr's state — §3.1.5 reads it at the path the round was given and §3.3.1 has
// already checked the claim's span against that text — so there is no standing
// to report and nothing to look up.
func readClaimProvenance(
	l state.Layout, owner, repo string, pr int, round *state.Meta, into *draft.Provenances,
) error {
	claims, notes, err := roundClaims(l, owner, repo, pr, round)
	if err != nil {
		return err
	}
	for i := range claims {
		switch claims[i].Source {
		case intent.ClaimFromNote:
			rests := draft.NoteClaim{Note: claims[i].NoteID, Standing: render.NoteMissing}
			if held, found := note.Find(notes, claims[i].NoteID); found {
				rests.Source, rests.Standing = string(held.Source), render.NoteStands
				if held.Retracted() {
					rests.Standing = render.NoteRetracted
				}
			}
			into.NoteClaims[claims[i].ID] = rests
		case intent.ClaimFromFile:
			into.FileClaims[claims[i].ID] = claims[i].File
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
	claims, notes, err := roundClaims(l, owner, repo, pr, round)
	if err != nil {
		return nil, err
	}
	withdrawn := make(map[string]bool, len(claims))
	for i := range claims {
		if claims[i].Source != intent.ClaimFromNote {
			continue
		}
		if !note.StandingOf(notes, claims[i].NoteID).Stands() {
			withdrawn[claims[i].ID] = true
		}
	}
	return withdrawn, nil
}

// roundClaims reads the round's claims and every note the issue key's store
// holds, which is what note.StandingOf needs whole.
//
// The claims are not filtered by source here, because the two callers want
// different subsets of them — §3.6.6's withdrawal reads the note-sourced ones
// and §8.1.6's region reads those and the file-sourced ones too — and one read
// of claims.ndjson per command is the point of the shared helper.
//
// A round that resolved no issue key has no store to load, so every
// note-sourced claim in it reads as resting on a note the store does not hold.
func roundClaims(
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
	return claims, notes, nil
}
