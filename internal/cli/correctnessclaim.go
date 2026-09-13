package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
)

// stampAxesAndClaims writes §6.1's `axis` row onto every record from §2.5.5's
// resolved corpus, and then applies §4.2.2's refusal, which reads that axis.
func stampAxesAndClaims(
	l state.Layout, owner, repo string, round int, file string, body []byte,
	pairs []mapping.Pair, records []*finding.Finding,
) error {
	corpus, err := role.Resolve(l.RepoRolesDir(owner, repo), l.RolesDir())
	if err != nil {
		return err
	}
	stampAxes(corpus, records)
	return refuseForeignClaims(file, body, round, pairs, records)
}

// refuseForeignClaims is the half of §4.2.2 cr can establish without judging: a
// correctness record whose `claim` names a claim id is refused with exit code 1
// unless the round's mapping maps that claim to the record's own unit.
//
// §4.2.1 evaluates a unit against the claims it is mapped to, so a correctness
// finding citing a claim mapped only elsewhere cites a claim its unit was never
// evaluated against — the finding reads as a violation of intent the author's
// change in this unit was not meant to implement. A correctness record naming
// no claim is kept: §4.2.3 still evaluates a unit for internal defects, and
// whether a finding violates a claim or is such a defect is a judgement §2.1.3
// leaves to the agent. Measured at 60b3370: refusing every correctness finding
// without a claim broke 20 tests for exactly that reason.
//
// It runs where every refusal that names an input line runs, before §6.4.4's
// drops shorten the slice the line numbers index.
func refuseForeignClaims(
	file string, body []byte, round int, pairs []mapping.Pair, records []*finding.Finding,
) error {
	at := state.RecordLines(body)
	for i, record := range records {
		if record.Axis != axis.Correctness || record.Claim == "" || mappedTo(pairs, round, record.Claim, record.Unit) {
			continue
		}
		return &finding.RejectedRecordError{
			File: file, Line: at[i], Field: "claim",
			Problem: fmt.Sprintf(
				"record %s cites %q, which this round's mapping does not map to unit %q; §4.2.2 has a correctness "+
					"finding cite a claim its own unit is evaluated against, so map the claim to %s with cr map record, "+
					"cite that unit's own claim, or name no claim for an internal defect (§4.2.3)",
				record.ID, record.Claim, record.Unit, record.Unit),
		}
	}
	return nil
}

// mappedTo reports whether the round's mapping joins claim to unit.
func mappedTo(pairs []mapping.Pair, round int, claim, unit string) bool {
	for i := range pairs {
		if pair := &pairs[i]; pair.Round == round && pair.Claim == claim && pair.Unit == unit {
			return true
		}
	}
	return false
}
