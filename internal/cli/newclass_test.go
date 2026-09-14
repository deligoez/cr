package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// summaryNewClassesOf reads §7.3.3's list back out of the round's summary.json.
func summaryNewClassesOf(t *testing.T, layout state.Layout) []string {
	t.Helper()
	body, err := layout.ReadRound(draftOwner, draftRepo, draftPRNum, draftRound, state.FileSummary)
	require.NoError(t, err)
	var summary struct {
		NewClasses []string `json:"new_classes"`
	}
	require.NoError(t, json.Unmarshal(body, &summary))
	return summary.NewClasses
}

// draftedNewClasses is what the `cr draft` run reported under §7.3.3.
func draftedNewClasses(t *testing.T) []string {
	t.Helper()
	printed, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)
	var reported struct {
		NewClasses []string `json:"new_classes"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &reported))
	return reported.NewClasses
}

// reclass rewrites the round's one record with a different class, which is the
// reword §7.3.3 is about: the id, the anchor and the prose are the same piece
// of work, and only the agent-composed slug moved.
func reclass(t *testing.T, layout state.Layout, id, class string) {
	t.Helper()
	record := aStoredRecord(id, finding.StateQueued)
	record.Class = class
	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.NoError(t, state.ReplaceStamped(
		held, state.FileFindings,
		state.Stamp{Head: draftHead, Round: draftRound},
		[]*finding.Finding{record},
	))
	require.NoError(t, held.Unlock())
}

// §7.3.3 through the command: a reworded class slug is reported as new, in the
// round summary and in `cr stats`.
//
// The fixture gives the repository a history first. An earlier pull request
// already raised `unchecked-error`, so when this round raises the same slug
// nothing is reported — which is the half that makes the other half mean
// something: a report that fired on every class would fire on a reword too,
// and prove nothing about drift.
//
// Then the slug is reworded to `dropped-error` on the same record, at the same
// anchor, with the same prose. Nothing about the finding changed except the key
// §7.4.1's waiver and §9.3.6's posted index are built from — which is exactly
// the silence §7.3.3 exists to break — and both required reports name it.
func TestARewordedClassSlugIsReportedAsNewInTheSummaryAndInStats(t *testing.T) {
	record := aStoredRecord("f1", finding.StateDraft)
	layout := draftedHome(t, record)

	// The repository's earlier history, on another pull request: this is
	// what makes `unchecked-error` an old class by the time round 2 of
	// pull request 7 raises it.
	earlier := aStoredRecord("e1", finding.StateDraft)
	require.NoError(t, finding.RecordRaised(layout, draftOwner, draftRepo,
		[]*finding.Finding{earlier}, &finding.TriageOccasion{
			PR: 3, Round: 1, Head: "0a1b2c3",
			At: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		}))

	assert.Empty(t, draftedNewClasses(t),
		"§7.3.3: a class the repository already raised is not new")
	assert.Empty(t, summaryNewClassesOf(t, layout))

	reclass(t, layout, "f1", "dropped-error")

	assert.Equal(t, []string{"dropped-error"}, draftedNewClasses(t),
		"§7.3.3: the reworded slug is a class the repository has never seen")
	assert.Equal(t, []string{"dropped-error"}, summaryNewClassesOf(t, layout),
		"§7.3.3: and the round summary is the first of the two places it is reported")

	printed, err := runCLIPrinting(t, "stats", "--repo", draftSlug)
	require.NoError(t, err)
	var report statsResult
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	assert.Equal(t, []finding.FirstSeen{
		{Class: "unchecked-error", PR: 3, Round: 1},
		{Class: "dropped-error", PR: draftPRNum, Round: draftRound},
	}, report.FirstSeen,
		"§7.3.3: and `cr stats` is the other, naming the occasion each was first seen on")
}

// A regeneration reports the same new classes the first rendering did.
//
// §7.1.6 lets a reviewer regenerate a round's draft as often as they like, and
// the second run reads a ledger that already carries the first run's `raised`
// events. A report computed against the whole ledger would call a class new
// once and never again, so the same round drafted twice would disagree with
// itself about what §7.3.3 owes the reader — and the run that disagreed would
// be the silent one.
func TestARegeneratedRoundReportsTheSameNewClasses(t *testing.T) {
	layout := draftedHome(t, aStoredRecord("f1", finding.StateDraft))

	first := draftedNewClasses(t)
	assert.Equal(t, []string{"unchecked-error"}, first,
		"the repository has no earlier history, so the round's only class is new")

	assert.Equal(t, first, draftedNewClasses(t), "§7.1.6: a second rendering says the same")
	assert.Equal(t, first, draftedNewClasses(t), "and a third")
	assert.Equal(t, first, summaryNewClassesOf(t, layout))
}

// §12.1's terminal shape for §7.3.3's report: a round that raised a class the
// repository has never seen names it on a line of its own, and a round that
// raised none prints no such line.
//
// The JSON document carries the list either way, so the tests above could not
// see the terminal line at all, and gremlins found both of its conditions
// unasserted: printed at an empty list, and printed only when empty.
func TestATerminalDraftNamesTheNewClassesOnlyWhenThereAreSome(t *testing.T) {
	t.Run("a class never seen", func(t *testing.T) {
		draftedHome(t, aStoredRecord("f1", finding.StateDraft))

		out := throughATerminal(t, "draft", draftPR, "--repo", draftSlug)

		assert.Contains(t, out, "\n§7.3.3: class(es) first seen in this round: unchecked-error\n")
	})
	t.Run("a class already raised", func(t *testing.T) {
		layout := draftedHome(t, aStoredRecord("f1", finding.StateDraft))
		require.NoError(t, finding.RecordRaised(layout, draftOwner, draftRepo,
			[]*finding.Finding{aStoredRecord("e1", finding.StateDraft)}, &finding.TriageOccasion{
				PR: 3, Round: 1, Head: "0a1b2c3",
				At: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
			}))

		out := throughATerminal(t, "draft", draftPR, "--repo", draftSlug)

		assert.NotContains(t, out, "§7.3.3", "no drift, so no drift report")
	})
}
