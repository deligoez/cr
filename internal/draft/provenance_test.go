package draft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// lookups are the provenances a round holds: one rule's rationale, and one
// claim resting on a note the context store holds with its source.
func lookups() *Provenances {
	return &Provenances{
		Rationales: map[string]string{"no-panic": "A panic takes the caller down."},
		NoteClaims: map[string]NoteClaim{"CR-7#c2": {Note: "CR-7#n1", Source: "chat"}},
	}
}

// cited is a record carrying the given citations under the no-panic rule.
func cited(citations ...finding.Citation) *finding.Finding {
	record := aRecord("f1")
	record.Rule, record.Citations = "no-panic", citations
	return record
}

// provenanceIn renders one record through Render with the round's lookups and
// returns the provenance region its comment carries, empty when none.
func provenanceIn(t *testing.T, record *finding.Finding) string {
	t.Helper()
	rendered, err := Render([]*finding.Finding{record}, render.LangEN, lookups())
	require.NoError(t, err)
	_, after, opened := strings.Cut(rendered, "<!-- cr:provenance -->\n")
	if !opened {
		return ""
	}
	inside, _, closed := strings.Cut(after, "\n<!-- cr:/provenance -->")
	require.True(t, closed, "a region that opens closes by its own pair")
	return inside
}

// §8.1.6's three triggers and round 9's both-citations case, through Render.
//
// Each trigger alone produces the region naming what it must; a record that
// meets none produces none. The both-citations case is the one round 9 settled:
// a record holding a rule-origin citation and an agent citation outside its
// unit is `cited` by either alone, and the region is emitted because a stored
// citation carries `origin: rule` — whatever else qualifies the record.
func TestEveryTriggerAndTheBothCitationsCaseDiscloseProvenance(t *testing.T) {
	ruled := finding.Citation{Path: "lib.go", Line: 4, Origin: finding.OriginRule}
	outside := finding.Citation{Path: "store.go", Line: 7, Origin: finding.OriginAgent}
	machine := aRecord("f1")
	machine.Rule, machine.Suggestion, machine.SuggestionOrigin = "no-panic", "\treturn err", finding.OriginRule
	onNote := aRecord("f1")
	onNote.Claim = "CR-7#c2"
	onIssue := aRecord("f1")
	onIssue.Claim = "CR-7#c1"
	for _, c := range []struct {
		name   string
		record *finding.Finding
		region string
	}{
		{"a machine suggestion", machine, "suggestion_origin: rule"},
		{"a claim resting on a note", onNote, "claim: CR-7#c2 (source: note)\nnote: CR-7#n1 (source: chat)"},
		{"a citation of rule origin", cited(ruled),
			"rule: no-panic\nrationale: A panic takes the caller down."},
		{"a rule citation beside an agent citation outside the unit", cited(outside, ruled),
			"rule: no-panic\nrationale: A panic takes the caller down."},
		{"an agent citation outside the unit alone", cited(outside), ""},
		{"a claim drawn from the issue", onIssue, ""},
		{"a record resting on nothing weak", aRecord("f1"), ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.region, provenanceIn(t, c.record))
		})
	}
}
