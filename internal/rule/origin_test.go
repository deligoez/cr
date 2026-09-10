package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/finding"
)

// §6.2.5: a citation is stamped `origin: rule` only when the ledger holds a hit
// for the same head, the same rule id as the record names, and the same path
// and line — and `origin: agent` in every case that differs in any one of
// them.
//
// Each case moves exactly one of the four halves of the match away from the
// hit the ledger holds, so what each proves is that one half is required. The
// last two keep all four and change what the ledger entry is: a record event
// or a dismissal at that place is cr's account of the agent, not of a
// detector, and supports nothing.
func TestACitationIsOfRuleOriginOnlyOnAFullPositionalMatch(t *testing.T) {
	for _, c := range []struct {
		name   string
		ledger Stat
		record *finding.Finding
		origin finding.Origin
	}{
		{"the hit's head, rule, path and line", hitWith(nil), citing("f1", "no-panic", 5), finding.OriginRule},
		{"another head", hitWith(func(s *Stat) { s.Head = "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c" }),
			citing("f1", "no-panic", 5), finding.OriginAgent},
		{"a real rule at a line it never hit", hitWith(nil), citing("f1", "no-panic", 4), finding.OriginAgent},
		{"another rule at the hit's line", hitWith(nil), citing("f1", "handle-every-error", 5), finding.OriginAgent},
		{"no rule at the hit's line", hitWith(nil), citing("f1", "", 5), finding.OriginAgent},
		{"the hit's line of another file", hitWith(func(s *Stat) { s.Path = "other.go" }),
			citing("f1", "no-panic", 5), finding.OriginAgent},
		{"a record event at the place", hitWith(func(s *Stat) { s.Event = EventRecord }),
			citing("f1", "no-panic", 5), finding.OriginAgent},
		{"a dismissal at the place", hitWith(func(s *Stat) { s.Event = EventDismissal }),
			citing("f1", "no-panic", 5), finding.OriginAgent},
	} {
		t.Run(c.name, func(t *testing.T) {
			StampOrigins([]Stat{c.ledger}, statsHead, []*finding.Finding{c.record})

			assert.Equal(t, c.origin, c.record.Citations[0].Origin)
		})
	}
}

// hitWith is the ledger's hit of no-panic on line 5 of lib.go at statsHead,
// with move applied to it when there is one.
func hitWith(move func(*Stat)) Stat {
	hit := Stat{Rule: "no-panic", Path: "lib.go", Line: 5, Event: EventHit, Head: statsHead}
	if move != nil {
		move(&hit)
	}
	return hit
}
