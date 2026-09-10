package draft

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// Provenances is what §8.1.6's provenance region needs from outside the
// records themselves: the rationale of every rule a record's citation was
// matched against, and the note every note-sourced claim rests on.
//
// The first and third triggers are read off the record — `suggestion_origin`
// and each citation's `origin` are fields cr stamped onto it — but what the
// region names for the second and third lives elsewhere: a note in the issue's
// context store (§3.6), and a rationale in the rule corpus (§2.6). The caller
// reads both and hands them here, so this package stays a renderer and reads
// no state of its own.
type Provenances struct {
	// Rationales are rule rationales by rule id.
	Rationales map[string]string
	// NoteClaims are the round's claims with `source: note`, by claim id.
	NoteClaims map[string]NoteClaim
}

// NoteClaim is the note one note-sourced claim rests on.
type NoteClaim struct {
	// Note is the claim's `note_id`.
	Note string
	// Source is that note's §3.6.3 source, empty when the context store no
	// longer holds the note.
	Source string
}

// of is §8.1.6's three triggers read over one record, with what each names.
//
// The third trigger is round 9's mechanical restatement: any stored citation
// carrying `origin: rule`, regardless of what else qualifies the record for
// `cited`. A record holding both a rule-origin citation and an agent citation
// outside its unit is `cited` by either alone, so asking which one it "takes
// its grade from" has two defensible answers; asking whether any citation is
// the rule's has one. The rule named is the record's own, because §6.2.5 only
// stamps `origin: rule` against a hit of the rule the record names.
//
// A nil receiver holds no lookups: the record's claim cannot then be told to
// rest on a note, and a rule-origin citation names the rule's id without its
// rationale. `cr draft` always hands one over; nil is for a caller whose
// records carry neither.
func (p *Provenances) of(record *finding.Finding) *render.Provenance {
	disclosed := &render.Provenance{Suggestion: record.SuggestionOrigin == finding.OriginRule}
	if p != nil && record.Claim != "" {
		if rests, found := p.NoteClaims[record.Claim]; found {
			disclosed.Claim, disclosed.Note, disclosed.NoteSource = record.Claim, rests.Note, rests.Source
		}
	}
	for at := range record.Citations {
		if record.Citations[at].Origin != finding.OriginRule {
			continue
		}
		disclosed.Rule = record.Rule
		if p != nil {
			disclosed.Rationale = p.Rationales[record.Rule]
		}
		break
	}
	return disclosed
}
