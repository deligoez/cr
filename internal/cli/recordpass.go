package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// refusePassedSeats refuses a file holding a record whose role and unit meet a
// `pass` cell of the current round, naming the record's line and id, the unit,
// the role and the cell.
//
// It is the other half of coverage.Decode's consistent check. That one refuses
// a `pass` cell recorded beside a record its role raised on its unit; this one
// refuses the same contradiction arriving in the opposite order, a record filed
// after the `pass` its role already recorded there. Without it the two orders
// would store different rounds from the same judgements, and `cr status` would
// count a complete row with a clean verdict over a unit the role raised a
// finding on.
//
// Like its counterpart it admits no exception, and it holds only this
// direction. A `finding` or `question` cell at a seat holding no record stays
// accepted, as TestAFindingCellWithNoSurvivingRecordIsAccepted pins: §6.4.4's
// waiver drop and §9.3.6's posted-index drop remove records and leave the cell
// the role filled when it raised them.
//
// The cells are read through state.ReadStamped, the round-scoped read
// roundCoverage gives `cr status`, so the seat this refuses at is a seat that
// command counts. It runs before dropRecorded: a record those drops would
// remove was still raised by its role, so the `pass` beside it is contradicted
// whether or not the record is stored.
func refusePassedSeats(
	l state.Layout, owner, repo string, pr, round int, file string, body []byte, records []*finding.Finding,
) error {
	cells, err := state.ReadStamped[coverage.Cell](l, owner, repo, pr, state.FileCoverage, round)
	if err != nil {
		return err
	}
	passed := make(map[coverage.Seat]bool, len(cells))
	for i := range cells {
		if cells[i].Result == coverage.ResultPass {
			passed[coverage.Seat{Unit: cells[i].Unit, Role: cells[i].Role}] = true
		}
	}
	at := state.RecordLines(body)
	for i, record := range records {
		if !passed[coverage.Seat{Unit: record.Unit, Role: record.Role}] {
			continue
		}
		return &finding.RejectedRecordError{
			File: file, Line: at[i], Field: "role",
			Problem: fmt.Sprintf(
				"is %q on unit %q in record %q, and round %d holds a %s cell at unit %q and role %q; "+
					"a role that found nothing on a unit raised no record there, so re-record that cell "+
					"as %s or %s with `cr cells record`, which replaces it, or drop the record",
				record.Role, record.Unit, record.ID, round, coverage.ResultPass, record.Unit, record.Role,
				coverage.ResultFinding, coverage.ResultQuestion),
		}
	}
	return nil
}
