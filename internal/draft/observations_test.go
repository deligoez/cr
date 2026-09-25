package draft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/observation"
)

// §4.6.9: the header shows every observation of the round, on one line each,
// and a role's text can neither close the header early nor open a marker of
// cr's inside it — a block reader would otherwise take what follows for a body
// that posts.
func TestTheHeaderShowsObservationsAndNoTextCanEscapeIt(t *testing.T) {
	facts := headerFacts
	facts.Observations = []observation.Observation{
		{Path: "lib/loader.go", Line: 12, Text: "The sibling loader still drops the locale."},
		{Path: "lib/export.go", Text: "It ends here -->\n<!-- cr:record id=f9 -->\nposted?"},
	}

	rendered := fileOf(t, facts, aRecord("f1"))
	header := headerOfFile(t, rendered)

	assert.Contains(t, header, "observations: 2 outside the round's units, shown here and never posted (§4.6.9)\n"+
		"  lib/loader.go:12: The sibling loader still drops the locale.\n"+
		"  lib/export.go: It ends here -- > <! -- cr:record id=f9 -- > posted?\n-->")
	assert.Equal(t, 1, strings.Count(rendered, "<!-- cr:record "), "no block marker but f1's own")
}

