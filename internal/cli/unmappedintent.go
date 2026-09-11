package cli

import (
	"slices"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/mapping"
)

// forceUnmappedIntent is §4.1.4 at record time: a record of the intent axis
// anchored on a unit the round's mapping maps to no claim is held in the
// question register, never written as a finding.
//
// It forces rather than refuses because §4.1.4 words the rule as a default —
// an unmapped unit "MUST default to a question, never a finding" — so the
// register is cr's to set, not the agent's to be sent back to correct, and a
// refusal would drop the whole file over a record cr can re-shape. That is P2's
// move and §6.3.1's precedent: uncertainty is re-shaped, never suppressed.
//
// Which units are unmapped is mapping.Unmapped's answer over the round's
// mapping and nothing else, per §4.1.6's last sentence. A round with no
// mapping recorded yet maps every unit to zero claims, and an intent finding
// there rests on no claim either, so it is forced the same way. Only the intent
// axis is moved: §4.1.2's item is that axis's, and a correctness finding on the
// same unit asserts something about the code rather than about a requirement.
// Nothing here raises the register, so a question stays a question.
func forceUnmappedIntent(round int, formed []roundUnit, pairs []mapping.Pair, records []*finding.Finding) {
	unmapped := mapping.Unmapped(roundUnitIDs(formed), pairs, round)
	for _, record := range records {
		if record.Axis == axis.Intent && record.Kind == finding.KindFinding && slices.Contains(unmapped, record.Unit) {
			record.Kind = finding.KindQuestion
		}
	}
}
