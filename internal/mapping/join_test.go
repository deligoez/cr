package mapping

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/state"
)

// pair is one stored line of mapping.ndjson, stamped with the round it was
// recorded in the way state.WriteStamped stamps it.
func pair(claim, unit string, round int) Pair {
	return Pair{Claim: claim, Unit: unit, Stamp: state.Stamp{Head: "abc123", Round: round}}
}

// A unit mapped to zero claims is what §4.1.2 raises, and a unit mapped to any
// number of claims is not.
//
// u1 carries two claims, which a derivation counting pairs rather than units
// would read as twice as mapped and a derivation keyed on the claim would read
// once; neither may raise it. u3 is mapped only by a pair of round 1, and round
// 2 is being asked about: §3.4.6 scopes a unit id to its round, so round 1's
// `u3` is a different piece of code, and a derivation over the whole file would
// hide round 2's unmapped `u3` behind history.
func TestAUnitMappedToZeroClaimsOfItsRoundIsUnmapped(t *testing.T) {
	pairs := []Pair{
		pair("CR-1#c1", "u1", 2),
		pair("CR-1#c2", "u1", 2),
		pair("CR-1#c3", "u3", 1),
		pair("CR-1#c3", "u4", 2),
	}

	assert.Equal(t, []string{"u2", "u3"}, Unmapped([]string{"u1", "u2", "u3", "u4"}, pairs, 2))
}

