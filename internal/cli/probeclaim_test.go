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

// §6.2.2 through `cr record`: a record whose probe was recorded at another head
// rests on an experiment this round cannot use, and is refused with exit 1
// naming the line, the field and both heads; findings.ndjson is untouched.
func TestARecordNamingAProbeFromAnotherHeadIsRefused(t *testing.T) {
	const elsewhere = "1f2e3d4c5b6a79880997a6b5c4d3e2f11f2e3d4c"
	layout := probedHome(t, gapFixture{
		result: "failed", head: elsewhere, passed: true, mapped: true, issue: gapIssue,
	})
	findings := layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings)
	before, err := os.ReadFile(findings)
	require.NoError(t, err)

	_, err = runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson", aProbedRecord(gapClaim)),
		"--repo", recordSlug)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, 1, rejected.Line)
	assert.Equal(t, "probe", rejected.Field)
	assert.Contains(t, rejected.Problem, elsewhere)
	assert.Contains(t, rejected.Problem, recordHead)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	after, err := os.ReadFile(findings)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "a refused file stores none of its records")
}

// §6.2.2's binding through `cr record`: the same supporting gap probe, named by
// a record whose anchor ends one line above the probe's target, is kept, graded
// argued, and asked as a question — the probe exists at this head, so the
// record is not refused, and it supports nothing.
func TestAProbeTargetOneLineOutsideTheAnchorLeavesTheRecordArgued(t *testing.T) {
	record := aProbedRecord(gapClaim)
	target := recordUnitStart[gapUnit] + 3
	record["anchor"] = map[string]any{
		"path": recordPath, "side": "RIGHT", "start_line": target - 2, "line": target - 1,
	}
	record["severity"] = "medium"

	answers, _ := recordedAnswers(t, gapFixture{
		result: "failed", passed: true, mapped: true, issue: gapIssue,
	}, record)

	require.Len(t, answers, 1)
	assert.False(t, answers[0].Supports, "§6.2.2: line "+strconv.Itoa(target)+" is outside the anchor range")
	assert.Contains(t, answers[0].Reason, "§6.2.2")
	layout := state.New(os.Getenv(state.HomeEnv))
	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, finding.GradeArgued, stored[0].Grade)
	assert.Equal(t, finding.KindQuestion, stored[0].Kind, "§6.3 asks an argued record as a question")
}
