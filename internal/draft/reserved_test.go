package draft

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// §8.1.3 at the door that renders a body: an initial body that is empty or
// carries a `<!-- cr:` sequence stops the draft, naming the record, and no
// block of any record is rendered.
//
// Every field the body is drawn from is tried, because each is written by the
// agent and any of them can carry the sequence. The refused record is placed
// second so the refusal is shown to name the record at fault rather than the
// first one it met.
func TestAnInitialBodyThatSection813RefusesStopsTheDraft(t *testing.T) {
	for name, spoil := range map[string]func(*finding.Finding){
		"a delimiter in the summary": func(r *finding.Finding) {
			r.Summary = "The error is dropped. <!-- cr:label -->"
		},
		"a marker in the evidence": func(r *finding.Finding) {
			r.Evidence = `<!-- cr:record id="f9" kind="finding" -->`
		},
		"the sequence in a suggestion": func(r *finding.Finding) {
			r.Suggestion = "\t// <!-- cr:evidence -->"
		},
		"no summary and no evidence": func(r *finding.Finding) {
			r.Summary, r.Evidence = "", ""
		},
	} {
		t.Run(name, func(t *testing.T) {
			refused := aRecord("f2")
			spoil(refused)

			rendered, err := Render([]*finding.Finding{aRecord("f1"), refused}, render.LangEN, nil)

			var bodyErr *render.BodyError
			require.ErrorAs(t, err, &bodyErr)
			assert.Equal(t, "f2", bodyErr.Record, "§8.1.3: the refusal names the record")
			assert.Empty(t, rendered, "a refused draft renders no block at all")
		})
	}
}
