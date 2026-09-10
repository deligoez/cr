package mapping

import "slices"

// Unmapped is §4.1.2's derivation over one round: the units, of those given,
// that the round's mapping maps to no claim, in the order they are given.
//
// It reads the mapping and nothing else, which is §4.1.6's last sentence — every
// rule of §4.1 through §4.4 that speaks of the claims a unit is mapped to reads
// this file — applied to the rule that speaks of a unit mapped to none. What
// the unit is about, what its hunks say, and what the claims say are not
// inputs: §4.1.6 makes the join the agent's judgement, and a derivation that
// looked past the pairs would be cr forming a second one.
//
// pairs is mapping.ndjson whole and round is the round being asked about,
// because §4.1.6 replaces the file for the current round only and earlier
// rounds stay in it (§9.3.5). A pair from round 1 naming `u2` says nothing about
// round 2's `u2`, which §3.4.6 makes a different piece of code.
func Unmapped(units []string, pairs []Pair, round int) []string {
	mapped := make(map[string]bool, len(pairs))
	for i := range pairs {
		if pairs[i].Round == round {
			mapped[pairs[i].Unit] = true
		}
	}
	unmapped := make([]string, 0, len(units))
	for _, id := range units {
		if !mapped[id] {
			unmapped = append(unmapped, id)
		}
	}
	return unmapped
}

// ClaimsOf is the other reading of the same join: the claim ids the round's
// mapping maps to one unit, in the order the pairs were recorded, each once.
//
// It is what §4.2.1 evaluates a unit against and what §4.6.1 carries into the
// unit's prompt as "the claims mapped to it". The order is the agent's own —
// the order it wrote the pairs in — because cr has no ranking of claims to put
// in its place, and a pair the file repeats is one mapping stated twice rather
// than a claim that counts double.
func ClaimsOf(pairs []Pair, round int, unit string) []string {
	claims := make([]string, 0)
	for i := range pairs {
		held := &pairs[i]
		if held.Round != round || held.Unit != unit || slices.Contains(claims, held.Claim) {
			continue
		}
		claims = append(claims, held.Claim)
	}
	return claims
}
