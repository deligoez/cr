package finding

import (
	"fmt"

	"github.com/deligoez/cr/internal/probe"
)

// GapBound is one of the two severity bounds §5.4 puts on a record resting on a
// gap probe: §5.4.4's floor and §5.4.5's ceiling.
//
// Its fields are unexported and the two values below are the only ones there
// are, so a bound cannot be invented at a call site and no third one can be
// declared without editing this file. §5.4 states two and no more: a failed gap
// probe that supports the finding carries severity at least `high`, and the
// results §5.4.5 names carry it at most `medium`.
type GapBound struct {
	// rule is what the bound requires, in the section's own words.
	rule string
	// allowed is the severities the bound leaves, named so the message
	// says what to write rather than only what is wrong.
	allowed []Severity
}

var (
	// GapAtLeastHigh is §5.4.4's floor: "Severity MUST be at least `high`
	// only when the probe supports the finding."
	//
	// The "only when" limits when the floor applies, not which way it
	// points, and nothing in §5.4.4 lowers it once it does. So a record a
	// gap probe supports is high or critical, and an unsupported one has
	// no floor at all — which is what keeps the crashed runner of round
	// 9's probe-ladder-asymmetry finding out of it: a signalled process is
	// `error` on §5.4.3's second rung, never `failed`, so it supports
	// nothing and reaches no floor.
	GapAtLeastHigh = GapBound{
		rule:    "§5.4.4 makes the severity of a supported failed gap probe at least high",
		allowed: []Severity{SeverityHigh, SeverityCritical},
	}
	// GapAtMostMedium is §5.4.5's ceiling: "Severity MUST be at most
	// `medium`."
	//
	// It is read over all five results §5.4.5 names rather than over
	// `passed` alone. The sentence has no subject of its own and the
	// nearest one is the paragraph's — "a record resting on one" of
	// `passed`, `timeout`, `error`, `no-tests-selected` and
	// `inconclusive` — and the reading matches what the section is for: a
	// record resting on a run that never finished, never started, or
	// selected nothing has no more behind it than one resting on a run
	// that showed the behaviour working.
	GapAtMostMedium = GapBound{
		rule: "§5.4.5 caps the severity of a record resting on a gap probe " +
			"that supports no probed grade at medium",
		allowed: []Severity{SeverityMedium, SeverityLow},
	}
)

// permits reports whether a severity satisfies the bound.
//
// Membership rather than comparison, and the difference matters at the edge: a
// `severity` cr does not recognise satisfies neither bound, so a record
// carrying one is refused wherever a bound applies rather than ranked into
// whichever half the comparison happened to put it. §6.1.3 checks the field's
// presence alone, so an unrecognised value does reach here.
func (b GapBound) permits(severity Severity) bool {
	for _, allowed := range b.allowed {
		if severity == allowed {
			return true
		}
	}
	return false
}

// GapSeverityError reports a record whose severity violates one of §5.4's two
// bounds, naming the record, the probe, and the result the bound was read from.
//
// Round 8's unenforced-severity-bound finding is why it exists. Severity is
// agent-written and §6.1.3 checks its presence alone, so both MUSTs bound
// nobody until a command refuses on them — and the cost of that is a
// misdirected human: severity is what the reviewer triages on under §1.6.2's
// blocking cap, so a probe that showed the behaviour working could present its
// record as the loudest item in the draft.
type GapSeverityError struct {
	// Record is the finding or question at fault.
	Record string
	// Probe is the gap probe it rests on.
	Probe string
	// Result is that probe's §5.5 `result`, which is what the bound was
	// read from.
	Result string
	// Severity is what the record carries.
	Severity Severity
	// Bound is the one it violates.
	Bound GapBound
	// By is the command that refused, so the same refusal at record time
	// and at post time says which moment it came from.
	By Actor
}

func (e *GapSeverityError) Error() string {
	return fmt.Sprintf(
		"%s: %s carries severity %q and rests on probe %s, which recorded %s: %s, so this "+
			"record must carry %s",
		e.By, e.Record, e.Severity, e.Probe, e.Result, e.Bound.rule, listedSeverities(e.Bound.allowed),
	)
}

// listedSeverities renders a bound's severities as "high or critical".
func listedSeverities(allowed []Severity) string {
	if len(allowed) == 2 {
		return string(allowed[0]) + " or " + string(allowed[1])
	}
	// Unreachable while both bounds allow two, and written rather than
	// assumed away: a bound added with one severity or three would
	// otherwise print a sentence that stops mid-clause.
	return fmt.Sprint(allowed)
}

// CheckGapSeverity holds one record to §5.4's severity bounds, and is the one
// enforcer of both.
//
// It is a call rather than a reading, which is what lets §7.2.2's shape work:
// the grade is re-checked after triage because triage changes the record, and
// severity is a field triage changes. `cr record` calls this before it stores
// anything and `cr post` calls it again immediately before the payload is
// built, and the two cannot come to different conclusions about the same
// record because there is one function and it takes the moment as an argument.
//
// gap is the probe record the finding rests on, and nil when it rests on none
// that grades this round — no `probe` field, a mutation probe, or a probe from
// another head, which §5.5.3 keeps out of the current round's grading. No probe
// means no bound: §5.4's two sentences are both about a record resting on a gap
// probe, and a record with other evidence is §6.2's business.
//
// supports is §5.4.4's answer for that probe, which probe.Supports computed
// from the resolved baseline and the round's mapping. It is passed rather than
// recomputed here because recomputing it would need the runs and the mapping,
// and a second reading of §5.4.4 that could disagree with the first is exactly
// what a bound this strict must not rest on.
func CheckGapSeverity(by Actor, record *Finding, gap *probe.Record, supports bool) error {
	bound, bounded := gapBoundFor(gap, supports)
	if !bounded || bound.permits(record.Severity) {
		return nil
	}
	return &GapSeverityError{
		Record: record.ID, Probe: gap.ID, Result: string(gap.Result),
		Severity: record.Severity, Bound: bound, By: by,
	}
}

// gapBoundFor is which of §5.4's bounds applies to a record resting on this
// probe, and whether either does.
//
// The unsupported `failed` probe is the case with no bound, and it is §5.4's
// own gap rather than an omission here. §5.4.4 gives the floor only to a probe
// that supports the finding and §5.4.5 speaks only of the five results it
// names, so a failed gap probe whose baseline was red, or whose finding names
// no mapped claim, is left to the severity the agent wrote — while §6.2 still
// leaves the record argued and §6.3 still asks it as a question.
func gapBoundFor(gap *probe.Record, supports bool) (GapBound, bool) {
	switch {
	case gap == nil:
		return GapBound{}, false
	case supports:
		return GapAtLeastHigh, true
	case !probe.Reproduces(gap):
		return GapAtMostMedium, true
	}
	return GapBound{}, false
}
