package cli

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
)

// §2.6.3.4 through `cr record`, `cr draft`'s triage ledger and
// `cr rules list --dead`: a clean round recorded after a rule's last hit is
// the newest round of the window, because `cr record` leaves the moment it
// recorded the round in the round summary.
//
// no-todo hit in pull request 7's round 1, and that pull request's round 2
// holds one triage event a year later. Pull request 13's round 2 is then
// recorded with no record at all, so nothing but its recording dates it. Audit
// round 14 row list-11-4 measured that round placed at time zero, sorted
// oldest and left out of a window of two, which then held both rounds of pull
// request 7 and kept no-todo alive. At rules.dead_after 2 the window is now
// pull request 13's round 2 and pull request 7's round 2, no-todo is silent
// across both, and it is reported dead.
func TestTheDeadWindowDatesACleanRoundByItsRecording(t *testing.T) {
	layout := recordedHome(t)
	require.NoError(t, os.WriteFile(layout.Rule("no-todo"), []byte(listedRuleJSON("no-todo")), 0o600))
	hit := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, rule.RecordHits(layout, recordOwner, recordRepo,
		[]rule.Hit{{RuleID: "no-todo", Path: "lib.go", Line: 4}},
		&rule.Occasion{PR: 7, Round: 1, Head: harvestHead, At: hit}))
	triaged := hit.AddDate(1, 0, 0)
	require.NoError(t, finding.RecordRaised(layout, recordOwner, recordRepo,
		[]*finding.Finding{{ID: "f9", Class: "unchecked-error"}},
		&finding.TriageOccasion{PR: 7, Round: 2, Head: harvestHead, At: triaged}))
	before := time.Now()
	_, err := runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson"), "--repo", recordSlug)
	require.NoError(t, err)
	after := time.Now()

	t.Setenv("CR_RULES_DEAD_AFTER", "2")
	var two rulesDeadResult
	listRules(t, &two, "--dead")

	require.Len(t, two.Window, 2)
	recorded := two.Window[0]
	assert.Equal(t, rule.LedgerRound{PR: recordPRNum, Round: recordRound, At: recorded.At, Dated: true}, recorded,
		"the clean round is the newest pair, dated by its recording")
	assert.WithinRange(t, recorded.At, before, after, "its moment is the one `cr record` ran at")
	assert.Equal(t, rule.LedgerRound{PR: 7, Round: 2, At: triaged, Dated: true}, two.Window[1])
	assert.True(t, two.Full)
	assert.Equal(t, map[string]string{"no-todo": "global"}, layersOf(two.Dead),
		"no-todo was silent across both rounds of the window")
}

// A round whose summary holds `cr record`'s counts and no recording moment —
// one recorded before `cr record` wrote it — has nothing in state that dates
// it, and is still ordered at its bound rather than dropped: the latest moment
// of an earlier round of its pull request, with dated false.
func TestARecordedRoundWithoutAMomentIsOrderedAtItsBound(t *testing.T) {
	layout := recordedHome(t)
	require.NoError(t, os.WriteFile(layout.Rule("no-todo"), []byte(listedRuleJSON("no-todo")), 0o600))
	hit := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, rule.RecordHits(layout, recordOwner, recordRepo,
		[]rule.Hit{{RuleID: "no-todo", Path: "lib.go", Line: 4}},
		&rule.Occasion{PR: recordPRNum, Round: recordRound - 1, Head: harvestHead, At: hit}))
	triaged := hit.AddDate(1, 0, 0)
	require.NoError(t, finding.RecordRaised(layout, recordOwner, recordRepo,
		[]*finding.Finding{{ID: "f9", Class: "unchecked-error"}},
		&finding.TriageOccasion{PR: 7, Round: 1, Head: harvestHead, At: triaged}))
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, state.UpdateRoundSection(held, recordRound, state.FileSummary, summaryDeduplicated, 0))
	require.NoError(t, state.UpdateRoundSection(held, recordRound, state.FileSummary, summarySuppressedByThread, 0))
	require.NoError(t, held.Unlock())

	t.Setenv("CR_RULES_DEAD_AFTER", "2")
	var two rulesDeadResult
	listRules(t, &two, "--dead")

	assert.Equal(t, []rule.LedgerRound{
		{PR: 7, Round: 1, At: triaged, Dated: true},
		{PR: recordPRNum, Round: recordRound, At: hit, Dated: false},
	}, two.Window)
	assert.True(t, two.Full)
	assert.Equal(t, map[string]string{"no-todo": "global"}, layersOf(two.Dead))
}

// A pull request's first round is a round like any other: recorded, with no
// rule hit and no triage event of its own, it enters the window dated by its
// recording.
//
// gremlins found this. Listing round directories from 2 up rather than from 1
// left every recorded first round out of the window unless a ledger dated it,
// so a repository's opening rounds could not count toward calling a rule dead.
func TestARecordedFirstRoundEntersTheDeadWindow(t *testing.T) {
	layout := recordedHome(t)
	recorded := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, state.UpdateRoundSection(held, 1, state.FileSummary, summaryDeduplicated, 0))
	require.NoError(t, state.UpdateRoundSection(held, 1, state.FileSummary, summarySuppressedByThread, 0))
	require.NoError(t, state.UpdateRoundSection(held, 1, state.FileSummary, summaryRecordedAt, recorded))
	require.NoError(t, held.Unlock())

	var printed rulesDeadResult
	listRules(t, &printed, "--dead")

	assert.Equal(t, []rule.LedgerRound{{PR: recordPRNum, Round: 1, At: recorded, Dated: true}}, printed.Window)
}
