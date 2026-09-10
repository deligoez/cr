package rule

import (
	"time"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// Event is one of §2.6.1.6's three kinds of entry in rule-stats.ndjson.
type Event string

const (
	// EventHit is a match §2.6.1.1's detection reported.
	EventHit Event = "hit"
	// EventRecord is a record `cr record` stored carrying a rule id.
	EventRecord Event = "record"
	// EventDismissal is a hit of the round no recorded record confirms,
	// which is §2.6.1.5's drop made observable.
	EventDismissal Event = "dismissal"
)

// Stat is one entry of rule-stats.ndjson: §2.6.1.6's rule id, matched path and
// line, round and head, plus the pull request and a UTC timestamp.
//
// The path and line are what the ledger is for. §6.2.5 stamps a citation
// `origin: rule` only when this file holds a hit for the same head, the same
// rule id, and the same path and line, so an entry carrying the rule id alone
// would let any citation claim any hit. The pull request and the timestamp are
// round 8's cross-pr-round-ordering-undefined: a round index is per pull
// request, so entries are grouped by the one and ordered by the other.
type Stat struct {
	// Rule is the rule id, per §2.6 item 3.
	Rule string `json:"rule"`
	// Path and Line are the matched location: the hit's for a hit or a
	// dismissal, and the record's anchor for a record.
	Path string `json:"path"`
	Line int    `json:"line"`
	// Event is which of the three this entry is.
	Event Event `json:"event"`
	// Record is the id of the record a record event was written for, and
	// empty on the other two, which no record stands behind.
	Record string `json:"record,omitempty"`
	// PR, Round and Head are the occasion the entry was written for.
	PR    int    `json:"pr"`
	Round int    `json:"round"`
	Head  string `json:"head"`
	// At is when it was written, in UTC.
	At time.Time `json:"at"`
}

// Occasion is the pull request, round, head and moment one command writes its
// ledger entries for.
type Occasion struct {
	PR    int
	Round int
	Head  string
	At    time.Time
}

// stat is one entry written on this occasion.
func (o *Occasion) stat(event Event, rule, path string, line int, record string) Stat {
	return Stat{
		Rule: rule, Path: path, Line: line, Event: event, Record: record,
		PR: o.PR, Round: o.Round, Head: o.Head, At: o.At.UTC(),
	}
}

// holds reports whether an entry was written for this occasion's pull request,
// round and head, whenever it was written.
func (o *Occasion) holds(s *Stat) bool {
	return s.PR == o.PR && s.Round == o.Round && s.Head == o.Head
}

// statKey is round 9's rule-stats-event-producer key, under which an entry is
// overwritten rather than appended: the rule id, path, line, round, head and
// event type, and the pull request and record id beside them. The pull request
// is there because a round index means nothing across pull requests, and the
// record id because two records of one rule anchored on one line are two
// records, not one written twice.
type statKey struct {
	rule, path, head, record string
	line, round, pr          int
	event                    Event
}

func (s *Stat) key() statKey {
	return statKey{
		rule: s.Rule, path: s.Path, head: s.Head, record: s.Record,
		line: s.Line, round: s.Round, pr: s.PR, event: s.Event,
	}
}

// HitStats are the ledger entries for the hits one detection run reported.
func HitStats(hits []Hit, on *Occasion) []Stat {
	stats := make([]Stat, 0, len(hits))
	for i := range hits {
		stats = append(stats, on.stat(EventHit, hits[i].RuleID, hits[i].Path, hits[i].Line, ""))
	}
	return stats
}

// RecordHits appends the hits one detection run reported to the repository's
// ledger, per §2.6.1.6. A hit already held for the same key is overwritten, so
// running detection twice in one round — `cr rules check` and then `cr review`,
// or either one twice — leaves one entry per hit rather than two.
func RecordHits(l state.Layout, owner, repo string, hits []Hit, on *Occasion) error {
	if len(hits) == 0 {
		return nil
	}
	return update(l, owner, repo, func(existing []Stat) []Stat {
		return merge(existing, HitStats(hits, on), nil)
	})
}

// RecordRecords appends what `cr record` owes the ledger, per §2.6.1.6 and
// round 9's rule-stats-event-producer: one record event for every stored
// record that carries a rule id, and one dismissal for every hit of the round
// that no record of the round confirms.
//
// recorded are the records this invocation stored; round are all of the
// round's records, those included, which is what a hit is confirmed against —
// a hit an earlier `cr record` of the same round confirmed is not dismissed by
// a later one that did not mention it.
//
// The round's dismissals are restated rather than accumulated. A dismissal
// says that no recorded record confirms a hit, which stops being true the
// moment one does, so every dismissal of this pull request, round and head is
// replaced by the set that holds now. Every other entry is carried through.
func RecordRecords(l state.Layout, owner, repo string, recorded, round []*finding.Finding, on *Occasion) error {
	return update(l, owner, repo, func(existing []Stat) []Stat {
		written := make([]Stat, 0)
		for _, record := range recorded {
			if record.Rule != "" {
				written = append(written, on.stat(
					EventRecord, record.Rule, record.Anchor.Path, record.Anchor.Line, record.ID))
			}
		}
		written = append(written, dismissals(existing, round, on)...)
		return merge(existing, written, func(s *Stat) bool {
			return s.Event == EventDismissal && on.holds(s)
		})
	})
}

// dismissals are the hits of this occasion's pull request, round and head that
// no record confirms, each once.
func dismissals(existing []Stat, round []*finding.Finding, on *Occasion) []Stat {
	dropped := make([]Stat, 0)
	seen := make(map[statKey]bool)
	for i := range existing {
		hit := &existing[i]
		if hit.Event != EventHit || !on.holds(hit) || confirmed(hit, round) {
			continue
		}
		dismissal := on.stat(EventDismissal, hit.Rule, hit.Path, hit.Line, "")
		if !seen[dismissal.key()] {
			seen[dismissal.key()] = true
			dropped = append(dropped, dismissal)
		}
	}
	return dropped
}

// confirmed reports whether any record confirms a hit: it names the hit's rule,
// per §2.6 item 3, and cites the hit's path and line, per §2.6.1.3 — the same
// positional match §6.2.5 stamps `origin: rule` by.
func confirmed(hit *Stat, records []*finding.Finding) bool {
	for _, record := range records {
		if record.Rule != hit.Rule {
			continue
		}
		for _, citation := range record.Citations {
			if citation.Path == hit.Path && citation.Line == hit.Line {
				return true
			}
		}
	}
	return false
}

// merge carries every existing entry through except those restated drops,
// overwrites in place an entry a written one shares a key with, and appends
// the written entries no existing one held.
func merge(existing, written []Stat, restated func(*Stat) bool) []Stat {
	at := make(map[statKey]int, len(existing))
	merged := make([]Stat, 0, len(existing)+len(written))
	for i := range existing {
		if restated != nil && restated(&existing[i]) {
			continue
		}
		at[existing[i].key()] = len(merged)
		merged = append(merged, existing[i])
	}
	for i := range written {
		if held, found := at[written[i].key()]; found {
			merged[held] = written[i]
			continue
		}
		at[written[i].key()] = len(merged)
		merged = append(merged, written[i])
	}
	return merged
}

// update is the ledger's one writer: state.UpdateRuleStats hands it the
// entries under the lock, and change decides what is published.
func update(l state.Layout, owner, repo string, change func([]Stat) []Stat) error {
	return state.UpdateRuleStats(l, owner, repo, change)
}

// ReadStats reads one repository's ledger without taking the lock, per §2.3.2.
func ReadStats(l state.Layout, owner, repo string) ([]Stat, error) {
	return state.ReadRuleStatsRecords[Stat](l, owner, repo)
}
