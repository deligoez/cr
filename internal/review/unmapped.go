// Package review carries the fan-out of spec/0.1.0.md §4.6 and the items the
// review axes of §4 raise into it.
//
// It judges nothing. §4.6 says of `cr review` that it "performs no judgement of
// its own and calls no model", and invariant 1 says the same of cr as a whole:
// what this package does is locate, attach, and shape, so an agent reading a
// prompt is handed an already-located candidate rather than asked to search.
package review

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/mapping"
)

// UnmappedUnit is §4.1.2's unmapped-unit item: a unit of the round the mapping
// maps to zero claims, raised in the register §4.1.4 fixes for it.
//
// The register travels with the item rather than being left to the role that
// answers it, because §4.1.4 says the item "MUST default to a question, never a
// finding" and gives the reason: the most common cause of an unmapped unit is
// intent that never reached the tracker, and an assertion that the author wrote
// code nobody asked for is exactly the wrong comment a colleague remembers.
// That is P2 at the source. §6.3's forcing reaches only a record graded
// `argued`, so a unit whose question happened to carry a citation outside the
// unit would clear §6.3 and go out as an assertion; the default here does not
// wait for a grade.
type UnmappedUnit struct {
	// Unit is the unit id of §3.4.6 the mapping maps to no claim.
	Unit string `json:"unit"`
	// Kind is the register the item is raised in, which §4.1.4 fixes at
	// finding.KindQuestion.
	Kind finding.Kind `json:"kind"`
}

// Unmapped raises §4.1.2's items over one round: one per unit the round's
// mapping maps to zero claims, in the order the units are given, each a
// question per §4.1.4.
//
// Which units are unmapped is mapping.Unmapped's answer and not this package's.
// §4.1.6 makes mapping.ndjson the one join every rule of §4.1 through §4.4
// reads, so this raises what that derivation found and adds only the register.
// The list is empty rather than nil when every unit is mapped, per §12.
func Unmapped(units []string, pairs []mapping.Pair, round int) []UnmappedUnit {
	ids := mapping.Unmapped(units, pairs, round)
	items := make([]UnmappedUnit, 0, len(ids))
	for _, id := range ids {
		items = append(items, UnmappedUnit{Unit: id, Kind: finding.KindQuestion})
	}
	return items
}
