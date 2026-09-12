package finding

import (
	"slices"
)

// FirstSeen is one class and the occasion §7.3.3 reports it as new on: the
// pull request and round the repository's ledger first held it.
//
// The class is agent-composed, so a reworded slug is a new class by every key
// cr has — §7.4.1's waiver key and §9.3.6's posted index both carry it — and a
// class that appears for the first time long after the review history started
// is what drift looks like from outside. Naming the occasion is what makes the
// report usable: "first seen on PR 31 round 2" is a place a reader can go and
// compare against the slug that was in use before it.
type FirstSeen struct {
	// Class is the class the occasion belongs to.
	Class string `json:"class"`
	// PR and Round are the occasion. They are reported as a pair because
	// a round index is per pull request, so the round alone names nothing.
	PR    int `json:"pr"`
	Round int `json:"round"`
}

// FirstSeenClasses is every class the ledger holds, with the occasion it was
// first seen on, in the order the ledger first held each.
//
// The order is the file's rather than the timestamp's, and that is the one
// decision here worth stating. §7.3.1 has a re-run overwrite an event under its
// key, which rewrites that event's `at` to the moment of the re-run — so a
// reviewer who regenerates an old round's draft would move that round's
// timestamps past every later round's and hand "first seen" to a round that
// merely happened second. The file's order is what the overwrite leaves alone:
// mergeTriageEvents replaces an event in place and appends only what is new, so
// position is the order the repository actually learned each fact.
func FirstSeenClasses(events []TriageEvent) []FirstSeen {
	seen := make(map[string]bool, len(events))
	first := make([]FirstSeen, 0, len(events))
	for i := range events {
		if seen[events[i].Class] {
			continue
		}
		seen[events[i].Class] = true
		first = append(first, FirstSeen{
			Class: events[i].Class, PR: events[i].PR, Round: events[i].Round,
		})
	}
	return first
}

// NewClasses are the classes among records that the ledger has never held on
// any occasion but this one — §7.3.3's report for one round, asked of the
// ledger held before the round's own events are written.
//
// Excluding this pull request and round is what makes the answer survive
// §7.1.6's regeneration. The round's second `cr draft` reads a ledger that
// already carries its own first run's events, and a report that counted those
// would call a class new once and never again — so the summary of a
// regenerated round would disagree with the summary of the same round drafted
// once, which is the one comparison a reviewer might actually make.
//
// The result is sorted, so the same round reports the same way twice (§2.1.1),
// and it is never nil (§12.3).
func NewClasses(events []TriageEvent, pr, round int, records []*Finding) []string {
	elsewhere := make(map[string]bool, len(events))
	for i := range events {
		if events[i].PR == pr && events[i].Round == round {
			continue
		}
		elsewhere[events[i].Class] = true
	}
	fresh := make([]string, 0)
	for _, record := range records {
		if elsewhere[record.Class] || slices.Contains(fresh, record.Class) {
			continue
		}
		fresh = append(fresh, record.Class)
	}
	slices.Sort(fresh)
	return fresh
}
