package rule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// The repository and occasion the ledger cases write for.
const (
	statsOwner = "acme"
	statsRepo  = "api"
	statsHead  = "9a8b7c6d5e4f30211220314f5e6d7c8b9a807162"
)

// occasion is round 2 of pull request 13, at a moment given in a zone other
// than UTC so a case can tell whether the entry was normalised.
func occasion(minute int) *Occasion {
	return &Occasion{
		PR: 13, Round: 2, Head: statsHead,
		At: time.Date(2026, 9, 11, 9, minute, 0, 0, time.FixedZone("TRT", 3*60*60)),
	}
}

// threeHits are three hits of one rule on three lines of one file.
func threeHits() []Hit {
	return []Hit{
		{RuleID: "no-panic", Path: "lib.go", Line: 4, Text: `panic("one")`},
		{RuleID: "no-panic", Path: "lib.go", Line: 5, Text: `panic("two")`},
		{RuleID: "no-panic", Path: "lib.go", Line: 6, Text: `panic("three")`},
	}
}

// citing is a record naming rule and citing one line of lib.go, anchored there.
func citing(id, rule string, line int) *finding.Finding {
	return &finding.Finding{
		ID: id, Rule: rule,
		Anchor:    finding.Anchor{Path: "lib.go", StartLine: line, Line: line},
		Citations: []finding.Citation{{Path: "lib.go", Line: line}},
	}
}

// ledger reads the repository's entries back.
func ledger(t *testing.T, l state.Layout) []Stat {
	t.Helper()
	held, err := ReadStats(l, statsOwner, statsRepo)
	require.NoError(t, err)
	return held
}

// §2.6.1.6: a hit carries the rule id, the matched path and line, the round
// and the head, and round 8's pull request and UTC timestamp beside them.
func TestAHitEntryCarriesEveryFieldTheLedgerIsKeyedAndOrderedBy(t *testing.T) {
	l := state.New(t.TempDir())
	on := occasion(0)

	require.NoError(t, RecordHits(l, statsOwner, statsRepo, threeHits()[:1], on))

	assert.Equal(t, []Stat{{
		Rule: "no-panic", Path: "lib.go", Line: 4, Event: EventHit,
		PR: 13, Round: 2, Head: statsHead, At: on.At.UTC(),
	}}, ledger(t, l))
	assert.Equal(t, time.UTC, ledger(t, l)[0].At.Location(), "the timestamp is written in UTC")
}

// Round 9's rule-stats-event-producer: an entry is overwritten under its key
// rather than appended again, so detection run twice in one round leaves one
// entry per hit — and the second run's moment is the one kept.
func TestDetectionRunTwiceInOneRoundLeavesOneEntryPerHit(t *testing.T) {
	l := state.New(t.TempDir())

	require.NoError(t, RecordHits(l, statsOwner, statsRepo, threeHits(), occasion(0)))
	require.NoError(t, RecordHits(l, statsOwner, statsRepo, threeHits(), occasion(5)))

	held := ledger(t, l)
	require.Len(t, held, 3, "one entry per hit, not two")
	for _, entry := range held {
		assert.Equal(t, occasion(5).At.UTC(), entry.At, "the entry was overwritten in place")
	}
}

// A hit of a later round, or of another pull request, is a different entry:
// the key holds the round and the pull request, and only an identical key
// overwrites.
func TestAHitOfAnotherRoundOrPullRequestIsItsOwnEntry(t *testing.T) {
	l := state.New(t.TempDir())
	later, other := occasion(0), occasion(0)
	later.Round = 3
	other.PR = 14

	for _, on := range []*Occasion{occasion(0), later, other} {
		require.NoError(t, RecordHits(l, statsOwner, statsRepo, threeHits()[:1], on))
	}

	assert.Len(t, ledger(t, l), 3)
}

// Round 9's rule-stats-event-producer names `cr record` as the producer of a
// dismissal: one per hit of the round that no recorded record confirms. The
// record that confirms one is itself written as a record event, at its anchor.
func TestRecordingWritesARecordEventAndDismissesEveryUnconfirmedHit(t *testing.T) {
	l := state.New(t.TempDir())
	require.NoError(t, RecordHits(l, statsOwner, statsRepo, threeHits(), occasion(0)))

	confirmation := citing("f1", "no-panic", 4)
	require.NoError(t, RecordRecords(l, statsOwner, statsRepo,
		[]*finding.Finding{confirmation}, []*finding.Finding{confirmation}, occasion(1)))

	held := ledger(t, l)
	require.Len(t, held, 6)
	assert.Equal(t, Stat{
		Rule: "no-panic", Path: "lib.go", Line: 4, Event: EventRecord, Record: "f1",
		PR: 13, Round: 2, Head: statsHead, At: occasion(1).At.UTC(),
	}, held[3])
	assert.Equal(t, []int{5, 6}, linesOf(held, EventDismissal),
		"§2.6.1.5's drop, made observable: the two hits no record cites")
}

// A dismissal says no recorded record confirms a hit, which stops being true
// once one does. A later `cr record` of the same round that confirms it takes
// its dismissal back, and one that mentions it not at all leaves a hit an
// earlier record confirmed alone.
func TestALaterConfirmationInTheSameRoundTakesTheDismissalBack(t *testing.T) {
	l := state.New(t.TempDir())
	require.NoError(t, RecordHits(l, statsOwner, statsRepo, threeHits(), occasion(0)))
	first := citing("f1", "no-panic", 4)
	require.NoError(t, RecordRecords(l, statsOwner, statsRepo,
		[]*finding.Finding{first}, []*finding.Finding{first}, occasion(1)))

	second := citing("f2", "no-panic", 5)
	require.NoError(t, RecordRecords(l, statsOwner, statsRepo,
		[]*finding.Finding{second}, []*finding.Finding{first, second}, occasion(2)))

	held := ledger(t, l)
	assert.Equal(t, []int{6}, linesOf(held, EventDismissal))
	assert.Equal(t, []int{4, 5}, linesOf(held, EventRecord))
	assert.Equal(t, []int{4, 5, 6}, linesOf(held, EventHit), "hits are never taken back")
}

// A hit is confirmed only by a record naming its rule and citing its path and
// line. The rule id alone would let a record about anything confirm every hit
// of that rule, and the location alone would let a record about another
// standard confirm this one.
func TestAHitIsConfirmedOnlyByItsRuleAtItsLocation(t *testing.T) {
	hit := &Stat{Rule: "no-panic", Path: "lib.go", Line: 5, Event: EventHit}
	elsewhere := citing("f1", "no-panic", 5)
	elsewhere.Citations[0].Path = "other.go"

	for _, c := range []struct {
		name      string
		record    *finding.Finding
		confirmed bool
	}{
		{"the rule at the line", citing("f1", "no-panic", 5), true},
		{"the rule at another line", citing("f1", "no-panic", 4), false},
		{"the rule at the line of another file", elsewhere, false},
		{"another rule at the line", citing("f1", "handle-every-error", 5), false},
		{"no rule at the line", citing("f1", "", 5), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.confirmed, confirmed(hit, []*finding.Finding{c.record}))
		})
	}
}

// Every write carries the entries it does not own through unchanged: another
// pull request's dismissals, another round's hits and records, and the entries
// of this round a write does not key. Only the round's own dismissals are
// restated, and only by `cr record`.
func TestAWriteCarriesEveryEntryItDoesNotOwnThroughUnchanged(t *testing.T) {
	l := state.New(t.TempDir())
	other, earlier := occasion(0), occasion(0)
	other.PR, earlier.Round = 14, 1
	require.NoError(t, RecordHits(l, statsOwner, statsRepo, threeHits(), other))
	require.NoError(t, RecordRecords(l, statsOwner, statsRepo, nil, nil, other))
	require.NoError(t, RecordHits(l, statsOwner, statsRepo, threeHits(), earlier))
	stale := citing("f1", "no-panic", 4)
	require.NoError(t, RecordRecords(l, statsOwner, statsRepo,
		[]*finding.Finding{stale}, []*finding.Finding{stale}, earlier))
	before := ledger(t, l)

	require.NoError(t, RecordHits(l, statsOwner, statsRepo, threeHits(), occasion(9)))
	confirmation := citing("f1", "no-panic", 6)
	require.NoError(t, RecordRecords(l, statsOwner, statsRepo,
		[]*finding.Finding{confirmation}, []*finding.Finding{confirmation}, occasion(9)))

	assert.Equal(t, before, ledger(t, l)[:len(before)],
		"no entry of another pull request or round was touched")
}

// linesOf lists the lines of one event's entries, in ledger order.
func linesOf(held []Stat, event Event) []int {
	lines := make([]int, 0)
	for i := range held {
		if held[i].Event == event {
			lines = append(lines, held[i].Line)
		}
	}
	return lines
}
