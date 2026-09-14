package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// QA follow-up of qa-cells-record-validation: `cr cells record` refuses an `na`
// cell at a seat holding the round's records, and `cr record` refuses the same
// contradiction in the other order — a record filed where its role already
// recorded `na` — as it refuses one filed at a `pass` cell. Measured before
// this: refusePassedSeats read `pass` cells only, so the record was stored and
// the seat said both "nothing to say" and a finding.
//
// The refusal is asserted whole, and findings.ndjson is byte-identical
// afterwards, not even the good f2 above the refused line stored. The control
// keeps the pass wording the other cell is refused with.
func TestARecordMeetingAnNACellOfItsRoleOnItsUnitIsRefused(t *testing.T) {
	for result, cell := range map[string]struct {
		line, said string
	}{
		"na": {
			`{"unit":"u1","role":"correctness","result":"na","reason":"generated code"}`,
			"had nothing to say about",
		},
		"pass": {
			`{"unit":"u1","role":"correctness","result":"pass"}`,
			"found nothing on",
		},
	} {
		t.Run(result, func(t *testing.T) {
			layout := recordedHomeWithCells(t)
			recordPassCells(t, cell.line)
			findings := layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings)
			before, err := os.ReadFile(findings)
			require.NoError(t, err)
			require.NotEmpty(t, before)
			file := writeRecordFile(t, "second.ndjson", aRecord("f2", "u2"), aRecord("f3", "u1"))

			_, err = runRecord(t, recordPR, file, "--repo", recordSlug)

			var rejected *finding.RejectedRecordError
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, ExitValidation, exitCodeFor(err), "§11.2 codes a refused record 1")
			assert.Equal(t, file+` line 2: role is "correctness" on unit "u1" in record "f3", `+
				`and round 2 holds a `+result+` cell at unit "u1" and role "correctness"; `+
				`a role that `+cell.said+` a unit raised no record there, so re-record that cell `+
				"as finding or question with `cr cells record`, which replaces it, or drop the record",
				err.Error())
			after, err := os.ReadFile(findings)
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after), "a refused file writes nothing, not even its good f2")
		})
	}
}

// An `na` from another role on the record's unit is another seat, and stands
// beside the record.
func TestARecordBesideAnNACellAtAnotherSeatIsAccepted(t *testing.T) {
	layout := recordedHomeWithCells(t)
	recordPassCells(t, `{"unit":"u1","role":"convention","result":"na","reason":"generated code"}`)

	_, err := runRecord(t, recordPR, writeRecordFile(t, "second.ndjson", aRecord("f2", "u1")),
		"--repo", recordSlug)

	require.NoError(t, err)
	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	assert.Len(t, stored, 2)
}
