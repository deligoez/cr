package rule

import (
	"cmp"
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

// LedgerRound is one (pull request, round) pair rule-stats.ndjson holds
// entries for, with the latest moment any of them was written.
//
// The pair is the unit §2.6.3.4 counts, per round 8's
// cross-pr-round-ordering-undefined: a round index is per pull request, so
// pull request 7's round 3 and pull request 12's round 3 are two rounds.
type LedgerRound struct {
	PR    int       `json:"pr"`
	Round int       `json:"round"`
	At    time.Time `json:"at"`
}

// DeadReport is §2.6.3.4's answer over one repository's ledger.
type DeadReport struct {
	// Window is the last deadAfter pairs, newest first.
	Window []LedgerRound
	// Full is whether the ledger held deadAfter pairs. A rule is called
	// dead only across a full window: across fewer rounds, "no hit in the
	// last deadAfter rounds" is not something the ledger can establish.
	Full bool
	// Dead are the corpus rules with no hit and no record in the window,
	// in corpus order. It is empty whenever Full is false.
	Dead []Resolved
}

// Dead reports the rules of corpus that produced no hit and no record across
// the last deadAfter (pull request, round) pairs of ledger, ordered by the
// ledger's timestamp descending, per §2.6.3.4 and round 8's
// cross-pr-round-ordering-undefined.
//
// A pair's moment is the latest entry written for it, and pairs sharing a
// moment are ordered by pull request and then round, so the same ledger gives
// the same window per §2.1.1. A dismissal keeps no rule alive on its own:
// §2.6.3.4 names a hit and a record, and every dismissal restates a hit the
// window already holds.
func Dead(corpus []Resolved, ledger []Stat, deadAfter int) DeadReport {
	window := ledgerRounds(ledger)
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

// ledgerRounds is every pair the ledger holds, newest first.
func ledgerRounds(ledger []Stat) []LedgerRound {
	latest := make(map[[2]int]time.Time)
	for i := range ledger {
		key := [2]int{ledger[i].PR, ledger[i].Round}
		if at, seen := latest[key]; !seen || ledger[i].At.After(at) {
			latest[key] = ledger[i].At
		}
	}
	rounds := make([]LedgerRound, 0, len(latest))
	for key, at := range latest {
		rounds = append(rounds, LedgerRound{PR: key[0], Round: key[1], At: at})
	}
	slices.SortFunc(rounds, func(a, b LedgerRound) int {
		return cmp.Or(b.At.Compare(a.At), cmp.Compare(a.PR, b.PR), cmp.Compare(a.Round, b.Round))
	})
	return rounds
}
