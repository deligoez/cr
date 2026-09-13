package rule

import (
	"cmp"
	"maps"
	"slices"
	"time"
)

// String names a layer the way `cr rules list` reports it, per §11's "the
// layer each came from".
func (s Source) String() string {
	switch s {
	case RepoSource:
		return "repo"
	case GlobalSource:
		return "global"
	case ProfileSource:
		return "profile"
	}
	return "unknown"
}

// LedgerRound is one (pull request, round) pair of the repository, with the
// moment it is ordered by.
//
// The pair is the unit §2.6.3.4 counts, per round 8's
// cross-pr-round-ordering-undefined: a round index is per pull request, so
// pull request 7's round 3 and pull request 12's round 3 are two rounds.
type LedgerRound struct {
	PR    int       `json:"pr"`
	Round int       `json:"round"`
	At    time.Time `json:"at"`
	// Dated is whether At is a moment state holds for the round itself. A
	// round `cr record` ran for can leave none — no rule entry and no triage
	// event — and its At is then the latest moment of an earlier round of the
	// same pull request, or zero when there is none. That is a bound and not a
	// guess: a pull request's rounds are opened one after another, so a round
	// cannot precede the rounds before it.
	Dated bool `json:"dated"`
}

// DeadReport is §2.6.3.4's answer over one repository's rounds.
type DeadReport struct {
	// Window is the last deadAfter pairs, newest first.
	Window []LedgerRound
	// Full is whether the repository held deadAfter pairs. A rule is called
	// dead only across a full window: across fewer rounds, "no hit in the
	// last deadAfter rounds" is not something the state can establish.
	Full bool
	// Dead are the corpus rules with no hit and no record in the window,
	// in corpus order. It is empty whenever Full is false.
	Dead []Resolved
}

// Dead reports the rules of corpus that produced no hit and no record across
// the last deadAfter (pull request, round) pairs, ordered by moment descending,
// per §2.6.3.4 and round 8's cross-pr-round-ordering-undefined.
//
// The pairs are every pair ledger holds entries for and every pair in rounds,
// whether or not a rule hit in it: a round every rule was silent in is a round
// every rule was silent across, and leaving it out would keep a rule alive on
// rounds older than the window. A pair's moment is the latest dated one given
// for it. An undated pair is ordered at its bound, per LedgerRound.Dated,
// which can only place it older than it is — and since an undated round holds
// no rule entry, a window that reaches past it reaches a round that might keep
// a rule alive and never one that calls a rule dead.
//
// Pairs sharing a moment are ordered by pull request, then by round with the
// later round first, since that is the order a pull request's rounds happened
// in; so the same state gives the same window per §2.1.1. A dismissal keeps no
// rule alive on its own: §2.6.3.4 names a hit and a record, and every dismissal
// restates a hit the window already holds.
func Dead(corpus []Resolved, ledger []Stat, rounds []LedgerRound, deadAfter int) DeadReport {
	window := ledgerRounds(ledger, rounds)
	full := deadAfter > 0 && len(window) >= deadAfter
	window = window[:min(len(window), max(deadAfter, 0))]
	report := DeadReport{Window: window, Full: full, Dead: make([]Resolved, 0)}
	if !full {
		return report
	}
	inWindow := make(map[[2]int]bool, len(window))
	for _, pair := range window {
		inWindow[[2]int{pair.PR, pair.Round}] = true
	}
	alive := make(map[string]bool)
	for i := range ledger {
		entry := &ledger[i]
		if entry.Event != EventDismissal && inWindow[[2]int{entry.PR, entry.Round}] {
			alive[entry.Rule] = true
		}
	}
	for i := range corpus {
		if !alive[corpus[i].Rule.ID] {
			report.Dead = append(report.Dead, corpus[i])
		}
	}
	return report
}

// ledgerRounds is every pair the ledger and rounds hold, each dated or bounded
// per LedgerRound.Dated, newest first.
func ledgerRounds(ledger []Stat, rounds []LedgerRound) []LedgerRound {
	pairs := make(map[[2]int]LedgerRound, len(rounds))
	note := func(pr, round int, at time.Time, dated bool) {
		key := [2]int{pr, round}
		held := pairs[key]
		held.PR, held.Round = pr, round
		if dated && (!held.Dated || at.After(held.At)) {
			held.At, held.Dated = at, true
		}
		pairs[key] = held
	}
	for i := range rounds {
		note(rounds[i].PR, rounds[i].Round, rounds[i].At, rounds[i].Dated)
	}
	for i := range ledger {
		note(ledger[i].PR, ledger[i].Round, ledger[i].At, true)
	}
	ordered := slices.SortedFunc(maps.Values(pairs), func(a, b LedgerRound) int {
		return cmp.Or(cmp.Compare(a.PR, b.PR), cmp.Compare(a.Round, b.Round))
	})
	var bound time.Time
	for i := range ordered {
		if i > 0 && ordered[i-1].PR != ordered[i].PR {
			bound = time.Time{}
		}
		if !ordered[i].Dated {
			ordered[i].At = bound
		}
		if ordered[i].At.After(bound) {
			bound = ordered[i].At
		}
	}
	slices.SortFunc(ordered, func(a, b LedgerRound) int {
		return cmp.Or(b.At.Compare(a.At), cmp.Compare(a.PR, b.PR), cmp.Compare(b.Round, a.Round))
	})
	return ordered
}
