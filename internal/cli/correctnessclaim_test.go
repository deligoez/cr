package cli

import (
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// mappedClaimsHome is recordedHome with §4.1.6's mapping joining CR-1#c1 to u1 and
// CR-1#c2 to u2.
func mappedClaimsHome(t *testing.T) state.Layout {
	t.Helper()
	layout := recordedHome(t)
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	pair := func(claim, unit string) string {
		return `{"claim":"` + claim + `","unit":"` + unit + `","head":"` + recordHead +
			`","round":` + strconv.Itoa(recordRound) + `}` + "\n"
	}
	require.NoError(t, held.Write(state.FileMapping, []byte(pair("CR-1#c1", "u1")+pair("CR-1#c2", "u2"))))
	require.NoError(t, held.Unlock())
	return layout
}

// §4.2.2 through `cr record`: a correctness finding on u1 citing CR-1#c2, which
// the mapping joins to u2 only, is refused with exit 1 naming the record, the
// claim and the unit, and nothing is stored.
func TestACorrectnessFindingCitingAnotherUnitsClaimIsRefused(t *testing.T) {
	layout := mappedClaimsHome(t)
	elsewhere := aRecord("f1", "u1")
	elsewhere["claim"] = "CR-1#c2"
	findings := layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings)
	before, err := os.ReadFile(findings)
	require.NoError(t, err)

	_, err = runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson", elsewhere), "--repo", recordSlug)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, "claim", rejected.Field)
	for _, named := range []string{"f1", `"CR-1#c2"`, `"u1"`} {
		assert.Contains(t, rejected.Problem, named)
	}
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	after, err := os.ReadFile(findings)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

// A correctness finding citing its own unit's claim is stored, and so is one
// citing no claim at all: whether that one violates a claim or is an internal
// defect (§4.2.3) is the agent's judgement, not cr's.
func TestACorrectnessFindingCitingItsOwnClaimOrNoneIsStored(t *testing.T) {
	layout := mappedClaimsHome(t)
	own := aRecord("f1", "u1")
	own["claim"] = "CR-1#c1"
	internal := aRecord("f2", "u1")
	internal["class"] = "nil-dereference"

	_, err := runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson", own, internal), "--repo", recordSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	assert.Equal(t, "CR-1#c1", stored[0].Claim)
	assert.Empty(t, stored[1].Claim)
}
