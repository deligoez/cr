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

