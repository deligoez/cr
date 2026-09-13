package rule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// entry is one ledger entry of rule on pull request pr's round, written at
// minute past a fixed hour.
func entry(event Event, rule string, pr, round, minute int) Stat {
	return Stat{
		Rule: rule, Path: "lib.go", Line: 4, Event: event, PR: pr, Round: round,
		At: time.Date(2026, 9, 1, 10, minute, 0, 0, time.UTC),
	}
}

// corpusOf is a corpus of bare rules in the order given.
func corpusOf(ids ...string) []Resolved {
	corpus := make([]Resolved, 0, len(ids))
	for _, id := range ids {
		corpus = append(corpus, Resolved{Rule: Rule{ID: id}})
	}
	return corpus
}

// idsOf is a report's dead rules by id.
func idsOf(report DeadReport) []string {
	ids := make([]string, 0, len(report.Dead))
	for i := range report.Dead {
		ids = append(ids, report.Dead[i].Rule.ID)
	}
	return ids
}

// A pair's moment is its latest entry: pull request 7's round 1 has a record
// written after pull request 12's round 1, so it is the newer pair and the one
// a window of one keeps.
func TestAPairIsDatedByItsLatestEntry(t *testing.T) {
	ledger := []Stat{
		entry(EventHit, "a", 7, 1, 0),
		entry(EventHit, "b", 12, 1, 1),
		entry(EventRecord, "a", 7, 1, 2),
	}

	report := Dead(corpusOf("a", "b"), ledger, 1)

	assert.Equal(t, []LedgerRound{{PR: 7, Round: 1, At: ledger[2].At}}, report.Window)
	assert.Equal(t, []string{"b"}, idsOf(report))
}

// §2.6.3.4 names a hit and a record. A record alone keeps its rule alive, and a
// dismissal restating a hit outside the window does not.
func TestARecordKeepsARuleAliveAndADismissalDoesNot(t *testing.T) {
	ledger := []Stat{
		entry(EventHit, "dismissed", 7, 1, 0),
		entry(EventRecord, "recorded", 7, 2, 1),
		entry(EventDismissal, "dismissed", 7, 3, 2),
	}

	report := Dead(corpusOf("recorded", "dismissed"), ledger, 2)

	assert.True(t, report.Full)
	assert.Equal(t, []string{"dismissed"}, idsOf(report))
}

// A ledger shorter than the window, and a window of no rounds at all, call no
// rule dead: neither can establish the silence §2.6.3.4 asks about.
func TestAWindowTheLedgerCannotFillCallsNothingDead(t *testing.T) {
	ledger := []Stat{entry(EventHit, "a", 7, 1, 0)}

	for _, deadAfter := range []int{2, 0, -1} {
		report := Dead(corpusOf("a", "b"), ledger, deadAfter)
		assert.False(t, report.Full, "dead_after %d", deadAfter)
		assert.Empty(t, report.Dead, "dead_after %d", deadAfter)
		assert.NotNil(t, report.Dead)
	}
}

// Pairs sharing a moment are ordered by pull request and then round, so one
// ledger always gives one window per §2.1.1.
func TestPairsSharingAMomentAreOrderedByPullRequestThenRound(t *testing.T) {
	ledger := []Stat{
		entry(EventHit, "a", 12, 1, 0),
		entry(EventHit, "a", 7, 2, 0),
		entry(EventHit, "a", 7, 1, 0),
	}

	report := Dead(corpusOf("a"), ledger, 3)

	at := ledger[0].At
	assert.Equal(t, []LedgerRound{{7, 1, at}, {7, 2, at}, {12, 1, at}}, report.Window)
}

// A ledger holding exactly deadAfter pairs fills the window. Every round
// §2.6.3.4 asks about is there, so a rule silent across all of them is dead from
// that round on, and not from one round later.
func TestALedgerHoldingExactlyTheWindowFillsIt(t *testing.T) {
	ledger := []Stat{
		entry(EventHit, "a", 7, 1, 0),
		entry(EventRecord, "a", 7, 2, 1),
	}

	report := Dead(corpusOf("a", "b"), ledger, 2)

	assert.True(t, report.Full)
	assert.Equal(t, []string{"b"}, idsOf(report))
}

// §11's layer names, in §2.6 item 1's order.
func TestALayerIsNamedTheWayRulesListReportsIt(t *testing.T) {
	assert.Equal(t, []string{"repo", "global", "profile", "unknown"},
		[]string{RepoSource.String(), GlobalSource.String(), ProfileSource.String(), Source(9).String()})
}
