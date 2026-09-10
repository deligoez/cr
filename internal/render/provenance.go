package render

import (
	"fmt"
	"strings"
)

// Provenance is what §8.1.6 discloses about one record, one group of fields per
// trigger, each group empty when its trigger does not apply.
//
// The triggers are §8.1.6's three, restated mechanically by round 9's
// provenance-trigger-ambiguity: the record carries `suggestion_origin: rule`;
// it rests on a claim with `source: note`; or any stored citation carries
// `origin: rule`, whatever else qualifies the record for `cited`. Round 12's
// provenance-trigger-undecidable settles the composition: when triggers
// coincide, every applicable trigger's content is emitted, in §8.1.6's order.
type Provenance struct {
	// Suggestion is the first trigger: the suggestion is the machine's,
	// per §2.6.2.4.
	Suggestion bool
	// Claim is the second: the id of the claim with `source: note` the
	// record rests on, and Note and NoteSource the note it came from and
	// that note's §3.6.3 source. NoteSource is empty when the context store
	// no longer holds the note.
	Claim      string
	Note       string
	NoteSource string
	// Rule is the third: the rule id a stored citation's `origin: rule`
	// was matched against, and Rationale that rule's `rationale` per §2.6
	// item 4. Rationale is empty when the corpus no longer holds the rule.
	Rule      string
	Rationale string
}

// ProvenanceRegion renders §8.1.6's provenance region for one record as §8.1.3's
// second owned region, or the empty string when no trigger applies.
//
// It is template substitution, as ProbeEvidence is: each line is a field name
// §6.1, §3.3 or §3.6 already gives the value, followed by the value as cr
// stored it, so the author checks the disclosure against the state rather
// than against a phrasing cr chose, and nothing in it is prose cr composed —
// which is also what lets it stand in a body `render.lang` governs. A value
// that is absent contributes no line rather than a line saying so.
//
// Being cr-owned, the region is generated from the record and never read back:
// AgentRegion discards whatever stands between its pair. So a value carrying
// Reserved would end the region early when the draft is read back, and hand
// the rest to the agent's region as if the agent had written it. The rationale
// is the one free-text value here, written into a project-owned rule file, and
// a record whose region would carry the sequence is refused naming the record,
// under §8.1.3's rejection of a body holding it.
func ProvenanceRegion(record string, p *Provenance) (string, error) {
	lines := make([]string, 0, 4)
	if p.Suggestion {
		lines = append(lines, "suggestion_origin: rule")
	}
	if p.Claim != "" {
		lines = append(lines, "claim: "+p.Claim+" (source: note)")
		note := "note: " + p.Note
		if p.NoteSource != "" {
			note += " (source: " + p.NoteSource + ")"
		}
		lines = append(lines, note)
	}
	if p.Rule != "" {
		lines = append(lines, "rule: "+p.Rule)
		if p.Rationale != "" {
			lines = append(lines, "rationale: "+p.Rationale)
		}
	}
	if len(lines) == 0 {
		return "", nil
	}
	content := strings.Join(lines, "\n")
	if strings.Contains(content, Reserved) {
		return "", &BodyError{Record: record, Problem: fmt.Sprintf(
			"would carry %q in its §8.1.6 provenance region, which §8.1.3 reserves for "+
				"cr's own delimiters; edit the rule's rationale", Reserved)}
	}
	return provenanceRegion.wrap(content), nil
}
