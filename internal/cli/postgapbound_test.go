package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// gapBoundCase is one of §5.4's two bounds, with a severity it admits and one it
// refuses, over the gap probe fixture that puts the bound in force.
type gapBoundCase struct {
	name     string
	fixture  gapFixture
	admitted string
	refused  string
	// summary replaces the record's statement where §6.3 forces it to a
	// question, which §8.1.5 then requires to read as one.
	summary string
}

// gapBoundCases are §5.4.4's floor, which a supported failed gap probe puts at
// high, and §5.4.5's ceiling, which a passed one puts at medium.
var gapBoundCases = []gapBoundCase{
	{
		name:     "a supported failed gap probe and section 5.4.4's floor",
		fixture:  gapFixture{result: "failed", passed: true, mapped: true, issue: gapIssue},
		admitted: "high", refused: "medium",
	},
	{
		name:     "a passed gap probe and section 5.4.5's ceiling",
		fixture:  gapFixture{result: "passed", passed: true, mapped: true, issue: gapIssue},
		admitted: "medium", refused: "critical",
		summary: "Is the error Decode returns dropped?",
	},
}

// triagedGapRound records one record resting on the fixture's gap probe at a
// severity the bound admits, drafts it, and edits its marker's severity in
// draft.md to the one the bound refuses — which §7.2 accepts, because the row is
// freely editable.
//
// The round's head is recordCheckout's single commit, which has no merge base to
// diff against, so the round's hunks are stood in for: one hunk carrying the
// record's anchor, which keeps §8.4.1's position check out of the way.
func triagedGapRound(t *testing.T, tc *gapBoundCase) state.Layout {
	t.Helper()
	layout := probedHome(t, tc.fixture)
	standInDiff(t, []git.Hunk{{
		Path: recordPath, Side: git.Right, BaseStart: 1, BaseLines: 100, HeadStart: 1, HeadLines: 100,
	}})
	record := aProbedRecord(gapClaim)
	record["severity"] = tc.admitted
	if tc.summary != "" {
		record["summary"] = tc.summary
	}
	_, err := runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson", record), "--repo", recordSlug)
	require.NoError(t, err, "the bound admits the severity the record was stored at")
	_, err = runDraft(t, recordPR, "--repo", recordSlug)
	require.NoError(t, err)

	drafted := layout.RoundFile(recordOwner, recordRepo, recordPRNum, recordRound, state.FileDraft)
	body, err := os.ReadFile(drafted)
	require.NoError(t, err)
	edited := markerEdit(t, string(body), "f1",
		`severity="`+tc.admitted+`"`, `severity="`+tc.refused+`"`)
	require.NoError(t, os.WriteFile(drafted, []byte(edited), 0o600))
	return layout
}

// §5.4.4 and §5.4.5 through `cr post`: a severity moved outside the bound in
// draft.md is refused with exit code 1 naming the record, before the payload is
// reported and before anything is put in front of the call — with and without
// --confirm, because §11.2 runs validation before the gate.
//
// Without the re-check the triage edit is the whole attack: `cr record` refused
// the same severity, and the reviewer reaches it by typing one attribute.
func TestPostRefusesASeverityTriageMovedOutsideTheGapBound(t *testing.T) {
	for _, tc := range gapBoundCases {
		for name, flags := range map[string][]string{"dry run": nil, "confirmed": {"--confirm"}} {
			t.Run(tc.name+", "+name, func(t *testing.T) {
				layout := triagedGapRound(t, &tc)
				posted := layout.RoundFile(recordOwner, recordRepo, recordPRNum, recordRound, state.FilePosted)
				before, err := os.ReadFile(posted)
				require.NoError(t, err)

				printed, err := runPost(t, append([]string{recordPR, "--repo", recordSlug}, flags...)...)

				require.Error(t, err)
				var refused *finding.GapSeverityError
				require.ErrorAs(t, err, &refused, "§7.2.2's shape: the bound is asked again after triage")
				assert.Equal(t, [4]string{"f1", "p1", string(tc.fixture.result), tc.refused},
					[4]string{refused.Record, refused.Probe, refused.Result, string(refused.Severity)},
					"the refusal names the record, the probe, its result and the edited severity")
				assert.Equal(t, ExitValidation, exitCodeFor(err))
				assert.Empty(t, printed, "the run refused before it reported a payload")
				after, err := os.ReadFile(posted)
				require.NoError(t, err)
				assert.Equal(t, string(before), string(after), "nothing was put in front of the call")
			})
		}
	}
}

// The control: the same round with its severity left where the bound admits it
// builds the payload. Without it a `cr post` refusing every record resting on a
// gap probe would pass the test above.
func TestPostSendsASeverityTheGapBoundAdmits(t *testing.T) {
	for _, tc := range gapBoundCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.refused = tc.admitted
			triagedGapRound(t, &tc)

			_, err := runPost(t, recordPR, "--repo", recordSlug)

			require.NoError(t, err)
		})
	}
}

// §7.2.2's shape for §5.4's bounds, through both commands: `cr record` refuses a
// severity outside the bound, `cr post` refuses the same severity reached by a
// triage edit instead, and the two refusals are one answer — the same record,
// probe, result and bound — differing only in the command that gave it.
//
// One function taking the moment as an argument is what keeps the answers from
// drifting, and this is the test that it is actually called at both moments
// rather than only able to be.
func TestTheSameBoundIsCheckedAtRecordTimeAndAtPostTime(t *testing.T) {
	for _, tc := range gapBoundCases {
		t.Run(tc.name, func(t *testing.T) {
			probedHome(t, tc.fixture)
			record := aProbedRecord(gapClaim)
			record["severity"] = tc.refused
			_, err := runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson", record), "--repo", recordSlug)
			var atRecord *finding.GapSeverityError
			require.ErrorAs(t, err, &atRecord)

			triagedGapRound(t, &tc)
			_, err = runPost(t, recordPR, "--repo", recordSlug)
			var atPost *finding.GapSeverityError
			require.ErrorAs(t, err, &atPost)

			assert.Equal(t, finding.ActorRecord, atRecord.By)
			assert.Equal(t, finding.ActorPostConfirm, atPost.By)
			atPost.By = atRecord.By
			assert.Equal(t, atRecord, atPost, "everything but the moment is the same answer")
		})
	}
}
