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

// The units come back in the order they were given, which is §3.4.6's id order
// when the caller hands in the round's units: the derivation reorders nothing,
// so the same round raises its items in the same order every time (§2.1.1).
func TestUnmappedKeepsTheOrderTheUnitsWereGivenIn(t *testing.T) {
	assert.Equal(t, []string{"u9", "u2", "u5"}, Unmapped([]string{"u9", "u2", "u5"}, nil, 1))
}

// A round whose every unit is mapped raises nothing, and says so with an empty
// list rather than a nil one, per §12: a caller serialising the answer must
// print `[]`, which is the difference between "nothing is unmapped" and "the
// question was never asked".
func TestAFullyMappedRoundRaisesAnEmptyListNotANilOne(t *testing.T) {
	unmapped := Unmapped([]string{"u1"}, []Pair{pair("CR-1#c1", "u1", 1)}, 1)

	assert.NotNil(t, unmapped)
	assert.Empty(t, unmapped)
}

// The claims mapped to a unit are the round's pairs naming it, in the order the
// agent recorded them, each once.
//
// Every way a pair can fail to belong is present beside the ones that do: a
// pair of another unit, a pair of round 1 naming the same unit id, and a pair
// the file states twice. A unit's prompt listing a claim of another round would
// have the role evaluate code against intent the round never joined to it
// (§3.4.6, §9.3.5), and a repeated claim would be read as a second requirement.
func TestTheClaimsOfAUnitAreItsRoundsPairsInRecordedOrder(t *testing.T) {
	pairs := []Pair{
		pair("CR-1#c3", "u1", 2),
		pair("CR-1#c1", "u2", 2),
		pair("CR-1#c9", "u1", 1),
		pair("CR-1#c1", "u1", 2),
		pair("CR-1#c3", "u1", 2),
	}

	assert.Equal(t, []string{"CR-1#c3", "CR-1#c1"}, ClaimsOf(pairs, 2, "u1"))
	assert.Equal(t, []string{}, ClaimsOf(pairs, 2, "u5"), "a unit no pair names has none, and says so with []")
}
