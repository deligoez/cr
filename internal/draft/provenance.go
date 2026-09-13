package draft

import (
	"fmt"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/render"
)

// Provenances is what the owned regions need from outside the records
// themselves: for §8.1.6's provenance region, the rationale of every rule a
// record's citation was matched against and the note every note-sourced claim
// rests on; for §8.1.7's evidence region, the probe every `probed` record
// rests on and the cap on the input it carries.
//
// The first and third provenance triggers are read off the record —
// `suggestion_origin` and each citation's `origin` are fields cr stamped onto
// it — but what the region names for the second and third lives elsewhere: a
// note in the issue's context store (§3.6), and a rationale in the rule corpus
// (§2.6). A `probed` record carries only its probe's id, and the probe itself
// is in probes.ndjson (§5.5). The caller reads all of them and hands them here,
// so this package stays a renderer and reads no state of its own.
type Provenances struct {
	// Rationales are rule rationales by rule id.
	Rationales map[string]string
	// NoteClaims are the round's claims with `source: note`, by claim id.
	NoteClaims map[string]NoteClaim
	// Probes are probes.ndjson's records by probe id.
	Probes map[string]*probe.Record
	// MaxProbeInput is the resolved post.max_probe_input_bytes of §2.7,
	// handed in rather than read here for the reason HeaderFacts gives its
	// cap.
	MaxProbeInput int
}

// NoteClaim is the note one note-sourced claim rests on.
type NoteClaim struct {
	// Note is the claim's `note_id`.
	Note string
	// Source is that note's §3.6.3 source, empty when the store no longer
	// holds the note.
	Source string
	// Standing is whether the note still stands. `cr draft` and `cr post`
	// hand over every note-sourced claim, a withdrawn note's included, so
	// §8.1.6's region names the note and says it was withdrawn.
	Standing render.NoteStanding
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
// rest on a note, and a rule-origin citation has no rationale to quote, which
// is refused rather than drawn without it. `cr draft` always hands one over;
// nil is for a caller whose records carry neither.
func (p *Provenances) of(record *finding.Finding) (*render.Provenance, error) {
	disclosed := &render.Provenance{Suggestion: record.SuggestionOrigin == finding.OriginRule}
	if p != nil && record.Claim != "" {
		if rests, found := p.NoteClaims[record.Claim]; found {
			disclosed.Claim, disclosed.Note, disclosed.NoteSource = record.Claim, rests.Note, rests.Source
			disclosed.NoteStanding = rests.Standing
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
		if disclosed.Rationale == "" {
			return nil, &MissingRationaleError{Record: record.ID, Rule: record.Rule}
		}
		break
	}
	return disclosed, nil
}

// MissingRationaleError refuses a record whose §8.1.6 provenance region would
// name its rule without the rule's rationale.
//
// §2.6 item 4 has every record a rule produced able to quote the rule's
// `rationale`, and §8.1.6 has the region name the rule id with it. A rule the
// corpus no longer resolves at draft or post time — its file removed or
// renamed since `cr record`, or scoped out of the round's profile — leaves the
// id with nothing to quote, and a region drawn without the line would tell the
// author the violation and not the standard while saying nothing about the
// gap. So the draft stops, naming the record and the rule, as
// MissingProbeError stops a `probed` record with no probe beneath it.
type MissingRationaleError struct {
	// Record is the id of the record carrying the rule-origin citation.
	Record string
	// Rule is the rule id it names.
	Rule string
}

func (e *MissingRationaleError) Error() string {
	return fmt.Sprintf(
		"record %s cites a hit of rule %q, whose rationale the rule corpus no longer holds: "+
			"§8.1.6's provenance region must quote it per §2.6 item 4, so the comment cannot be rendered",
		e.Record, e.Rule)
}
