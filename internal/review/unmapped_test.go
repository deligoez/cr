package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/state"
)

// stored is one line of mapping.ndjson as the writer leaves it, stamped with the
// round the pair was recorded in.
func stored(claim, unit string, round int) mapping.Pair {
	return mapping.Pair{Claim: claim, Unit: unit, Stamp: state.Stamp{Head: "abc123", Round: round}}
}

// A unit mapped to zero claims raises an unmapped-unit item, and a unit mapped
// to one does not (§4.1.2).
func TestAUnitMappedToZeroClaimsRaisesAnUnmappedUnitItem(t *testing.T) {
	items := Unmapped([]string{"u1", "u2", "u3"}, []mapping.Pair{stored("CR-1#c1", "u2", 1)}, 1)

	require.Len(t, items, 2)
	assert.Equal(t, "u1", items[0].Unit)
	assert.Equal(t, "u3", items[1].Unit)
}

// An unmapped unit's record arrives as a question before §6.3's forcing runs.
//
// The forcing is the wrong thing to lean on, and this test is how that is
// kept visible. §6.3.1 moves a record to `kind: question` only when §6.2 graded
// it `argued`, so an unmapped-unit question that happened to cite a line
// outside its unit would be graded `cited`, pass §6.3 untouched, and reach the
// author asserting that they wrote code nobody asked for — which §4.1.4 rules
// out because the usual cause is intent that never reached the tracker.
//
// So the item is handed to §6.3 in both registers the grade can put it in, and
// §6.3 is asked what it did. finding.ForceQuestion reports false for a record
// that was already a question, and false is what it must report for each: the
// register came with the item, not from the forcing.
func TestAnUnmappedUnitArrivesAsAQuestionBeforeTheForcingRuns(t *testing.T) {
	items := Unmapped([]string{"u1"}, nil, 1)
	require.Len(t, items, 1)
	assert.Equal(t, finding.KindQuestion, items[0].Kind,
		"§4.1.4: an unmapped unit defaults to a question, never a finding")

	for _, grade := range []finding.Grade{finding.GradeArgued, finding.GradeCited, finding.GradeProbed} {
		record := finding.Finding{Unit: items[0].Unit, Kind: items[0].Kind, Grade: grade}

		assert.False(t, finding.ForceQuestion(&record),
			"a %s record raised from the item was already a question, so §6.3 moved nothing", grade)
		assert.Equal(t, finding.KindQuestion, record.Kind,
			"and a %s record, which §6.3 never forces, is a question all the same", grade)
	}
}

