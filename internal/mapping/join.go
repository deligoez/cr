package mapping

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
