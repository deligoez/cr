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

