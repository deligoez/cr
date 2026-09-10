package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §8.1.6: each trigger alone produces the region, between §8.1.3's
// provenance pair, naming what its trigger requires — the machine origin, the
// note and its source per §3.6.3, or the rule id with its rationale per §2.6
// item 4 — and a record no trigger applies to gets no region at all.
func TestEachTriggerAloneNamesWhatSection816Requires(t *testing.T) {
	for _, c := range []struct {
		name       string
		provenance Provenance
		region     string
	}{
		{"no trigger", Provenance{}, ""},
		{"a machine suggestion", Provenance{Suggestion: true},
			"<!-- cr:provenance -->\nsuggestion_origin: rule\n<!-- cr:/provenance -->"},
		{"a claim resting on a note", Provenance{Claim: "CR-7#c2", Note: "CR-7#n1", NoteSource: "chat"},
			"<!-- cr:provenance -->\nclaim: CR-7#c2 (source: note)\nnote: CR-7#n1 (source: chat)\n" +
				"<!-- cr:/provenance -->"},
		{"a citation of rule origin", Provenance{Rule: "no-panic", Rationale: "A panic takes the caller down."},
			"<!-- cr:provenance -->\nrule: no-panic\nrationale: A panic takes the caller down.\n" +
				"<!-- cr:/provenance -->"},
	} {
		t.Run(c.name, func(t *testing.T) {
			region, err := ProvenanceRegion("f1", &c.provenance)
			require.NoError(t, err)
			assert.Equal(t, c.region, region)
		})
	}
}
