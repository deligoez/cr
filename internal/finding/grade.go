package finding

import "github.com/deligoez/cr/internal/probe"

// Containment answers §6.2.1's containment question about one unit: is this
// `path:line` inside it?
//
// It is an interface for the reason HonestyDisclosure is one: the answer is
// unit.Unit's, whose hunk ranges are already recorded in the head coordinates
// §6.2.1 evaluates containment in, and internal/finding cannot import
// internal/unit — internal/unit reaches internal/profile, whose tests reach
// this package, so the import would close a cycle in three test binaries. The
// rule is stated once, where the coordinates live, and named here.
//
// Nothing an agent writes can implement it. The value that arrives is the
// record's own unit as §3.4.6 recorded it, read out of units.ndjson, which is
// `cr brief`'s to write — which is what §6.2.1's "no field the agent writes can
// move a location across the boundary" comes to at this end.
type Containment interface {
	// Contains reports whether path:line lies inside the unit, in head
	// coordinates and with no side compared.
	Contains(path string, line int) bool
}

// Evidence is §6.2.1's input set, as `cr` resolved it out of files the agent
// does not write.
//
// §6.2.1 closes the set: "The only inputs are `axis`, `unit`, `anchor`,
// `claim`, `citations`, the mapping of §4.1.6, and the referenced probe
// record". Five of those seven are rows of the record itself and travel with
// it. The other two are not on the record at all — the round's mapping and the
// stored probe — and this is where they arrive, already reduced to the answers
// §5.3.5 and §5.4.4 give.
//
// It is a value with unexported fields and one constructor, which is what makes
// the closure structural rather than remembered. There is nowhere here to put
// `evidence`, `summary`, `severity` or `role`, so the sentence "`evidence`
// prose is never parsed" is a thing this package cannot do rather than a thing
// it refrains from doing — and a caller cannot hand the grader a `probed`
// answer it preferred, because the only way to fill that field is to satisfy
// probe.Establishes with a Baseline and a ClaimMapping neither the agent nor
// the caller can manufacture.
type Evidence struct {
	// own is the record's own unit, as §3.4.6 recorded it. §6.2's `cited`
	// row asks whether a citation "lies outside the record's own unit",
	// and §6.2.1 answers that in head coordinates against the unit's hunk
	// ranges, which is the question Containment names.
	own Containment
	// probed is §6.2's `probed` row: the referenced probe record graded at
	// this head and its result supported the claim per §5.3 and §5.4.
	probed bool
}

// Resolved assembles the two inputs §6.2.1 names that the record does not carry.
//
// own is the unit the record sits on, read out of units.ndjson for the current
// round. referenced is the probe record the finding's `probe` field names, nil
// when it names none or names one no round holds; head is the round's head, and
// baseline and claim are §5.2.6's admitted run and §4.1.6's stored mapping as
// probe resolved them.
//
// The probe question is settled here and not at grading time, so the record and
// the answer travel together. A grader holding an Evidence cannot ask the probe
// a second question and cannot get a second answer.
func Resolved(
	own Containment, referenced *probe.Record, head string,
	baseline probe.Baseline, claim probe.ClaimMapping,
) Evidence {
	return Evidence{own: own, probed: probe.Establishes(referenced, head, baseline, claim)}
}

// inside is §6.2.1's containment question, asked of the record's own unit.
//
// A record whose unit did not resolve arrives with no Containment at all, and
// that is answered here rather than at the unit: nothing is inside a unit that
// is not there. It is the closed answer only because it cannot be reached
// through `cr record` — finding.Decode has already refused a record naming a
// unit the round does not hold — and saying so beside the check is what keeps
// the open answer from being reintroduced as a convenience.
func (e Evidence) inside(path string, line int) bool {
	return e.own != nil && e.own.Contains(path, line)
}

// testAxis is §1.5's `test` axis id, which §6.2's `cited` row names.
//
// It is a literal rather than internal/axis's own constant, and the reason is
// the build constraint Containment gives: internal/axis's tests reach
// internal/config, which reaches internal/render, which reaches this package,
// so importing the closed set would close a cycle in that test binary. What
// keeps the two spellings from drifting is a test — internal/finding's tests
// may import internal/axis, because nothing in that direction is a cycle — and
// it asserts they are one string.
const testAxis = "test"

// cites is §6.2's `cited` row read over one record: does it carry at least one
// entry `cr` resolved against the current head that either has `origin: rule`
// or lies outside the record's own unit, and is the record's `axis` not `test`?
//
// The axis is asked first, because it settles the row for the whole test axis
// before any citation is looked at. §4.4.2 is what the clause is for: a
// test-adequacy finding asserts only with an experiment, and citations to test
// files must not grade it `cited` — so a record on that axis is `probed` or
// `argued` and there is no third answer. Asserting that a unit is untested on
// the strength of having read the tests is exactly the claim §5.3's
// `no-test-failed` exists to settle, and a `cited` grade would let it be made
// without running anything.
//
// An axis cr did not compute is refused with it. §6.1 makes `axis` the axis of
// the record's `role`, written by cr, so an empty one is a role the corpus did
// not resolve rather than a record on some other axis — and the row asks cr to
// establish that the axis is not `test`, which it cannot do about an axis it
// never worked out. The direction is the safe one: it can only lower a grade to
// `argued`, which §6.3 asks as a question.
//
// "That `cr` resolved" is read off `content_hash` rather than assumed. §6.2.3
// has `cr` compute and store that hash for every entry that resolved, and
// §6.1.4 rejects a record arriving with one, so the field is present exactly
// where cr's own resolution put it. An entry without it is an entry no
// resolution ever reached, and grading it would be grading a location nobody
// has opened.
//
// The two branches are §6.2's own disjunction, and each closes a different
// hole. A citation inside the record's own unit is the code the record is
// already about, so it adds nothing a human could check the summary against —
// except when `cr`'s own rule machinery put it there, which is what
// `origin: rule` means and why §6.2.5 has cr stamp that field positionally
// rather than let the agent write it.
func (e Evidence) cites(record *Finding) bool {
	if record.Axis == "" || record.Axis == testAxis {
		return false
	}
	for i := range record.Citations {
		// Indexed rather than ranged by value: nothing here writes to
		// the entry, and it is read three times.
		citation := &record.Citations[i]
		if citation.ContentHash == "" {
			continue
		}
		if citation.Origin == OriginRule || !e.inside(citation.Path, citation.Line) {
			return true
		}
	}
	return false
}

// ComputeGrade is §6.2's table over one record: the grade `cr` computes from
// the record and the evidence it resolved, never the grade the agent asserted.
//
// The rows are tried in the table's own order, and the order is load-bearing
// rather than cosmetic. §6.2's third row is "neither of the above", so `argued`
// is what falls out when the first two do not match; a computation that tried
// `cited` first would still reach the same answer here, but the table would
// have to be read twice to see why.
//
// Nothing in this function can read a field the agent writes about its own
// standing. `grade` itself is refused on the wire by §6.1.4, `probe` reaches
// this only through the probe record cr resolved for it, and `citations` reach
// it only with the hash cr stamped.
func ComputeGrade(record *Finding, evidence Evidence) Grade {
	switch {
	case evidence.probed:
		return GradeProbed
	case evidence.cites(record):
		return GradeCited
	}
	return GradeArgued
}

// Regrade writes §6.2's grade onto the record, and is the only place the field
// is written.
//
// The write is a ratchet, which is round 13's mutable-grading-input finding.
// The mapping of §4.1.6 is a grading input that changes inside a round — §4.1.6
// replaces it and §3.3.1 clears it — while §6.3.1 recomputes the grade three
// times over the life of one round and §7.2 accepts a question becoming a
// finding on a recomputed `probed`. So a recomputation MUST NOT raise a
// record's grade within a round; it may only lower it or leave it unchanged.
// Otherwise a record that reached record time as `argued`, and was therefore
// forced to a question and shown to the human as one, could arrive at post time
// as an assertion because a claim was mapped to its unit in between — and the
// human would have approved a question.
//
// Lowering is allowed, and is not the mirror of the same risk. A record whose
// evidence stopped standing up says less than it did, and §6.3 has the weaker
// grade force it to a question; the direction the trust economy has to refuse
// is the one that turns a question into an assertion behind the reviewer.
//
// A record that has not been graded yet holds no grade for the ratchet to
// measure against, and the first computation simply stands. That is not the
// same as holding the weakest grade: rankIn ranks an unlisted value behind
// every grade there is, so treating an unset field as a rank would refuse
// `probed` and `cited` on the very first write and leave every record `argued`.
func Regrade(record *Finding, evidence Evidence) {
	record.Grade = notAbove(ComputeGrade(record, evidence), record.Grade)
}

// notAbove is the ratchet itself: the computed grade when it is no stronger
// than the one the record already holds, and the held one otherwise.
//
// It compares ranks in gradeStrength rather than the values, so it is the same
// order §6.4.2 picks a duplicate group's representative by. Two readings of
// "the highest grade" that could disagree is exactly what one shared order
// prevents.
//
// A held value gradeStrength does not list is a record with no grade to be
// raised above — an ungraded one, since §6.1.4 refuses the field on the wire
// and cr writes nothing else there — and the computed value takes it. rankIn
// gives that case the rank past the end, which is the one comparison that says
// so without a second spelling of §6.2's three.
func notAbove(computed, held Grade) Grade {
	settled, standing := rankIn(gradeStrength, computed), rankIn(gradeStrength, held)
	if standing == len(gradeStrength) || settled >= standing {
		return computed
	}
	return held
}
