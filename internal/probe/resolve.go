package probe

import (
	"fmt"

	"github.com/deligoez/cr/internal/run"
)

// Baseline is a run record §5.2.6 admits as a probe's baseline: at the current
// head, measuring the same tests, on unmutated and un-probed code.
//
// Its fields are unexported and Resolve is its only constructor, and that is
// the point of the type. §5.2.6's fence — "Only a run record carrying no
// `probe` may serve as a baseline" — is then not a check some caller has to
// remember to make, because there is no way to hold a Baseline that was built
// from a run carrying one. A probe's own mutated run can be read, printed and
// counted like any other record, and still cannot be handed to §5.3.5 or
// §5.4.4 as the thing that licenses a `probed` grade.
//
// Passed travels inside the value for the same reason. §5.3.5 and §5.4.4 ask
// whether "the probe's `baseline` run record has `passed: true`", and a grader
// holding a Baseline can only answer that about the record the resolution
// actually chose — not about some other run it happened to have in hand.
type Baseline struct {
	// id is the run record's §5.2.4 id, which is what §5.5's `baseline`
	// column stores.
	id string
	// passed is §5.2.5's verdict on that record, as it was written.
	passed bool
	// failed is the record's `tests_failed`, and counted whether §5.2.4
	// found it derivable, so a baseline that did not pass can be named with
	// the count it failed on.
	failed  int
	counted bool
}

// ID is the run record id §5.5's `baseline` column carries.
func (b Baseline) ID() string { return b.id }

// Passed is §5.2.5's verdict on the resolved record, which §5.3.5 and §5.4.4
// require before a probe may support a `probed` grade.
//
// It is deliberately not a condition of resolution. §5.2.5 explains that a run
// whose counts are underivable "cannot serve as a baseline", and the way that
// comes true is here rather than in Resolve: a repository whose suite is
// already red has a baseline, it is simply one that did not pass, and that is
// the fact §5.3.5 needs in order to refuse the grade. Refusing to resolve it
// would instead have §5.2.6 perform a fresh baseline before every probe for
// ever, resolve none of them, and leave the ladder with nothing to consult.
func (b Baseline) Passed() bool { return b.passed }

// Failed is the resolved record's `tests_failed`, and false when §5.2.4 found
// no failed count derivable. It is read for the report of a baseline that did
// not pass, and never for a verdict: Passed is §5.2.5's, whole.
func (b Baseline) Failed() (int, bool) { return b.failed, b.counted }

// baselineOf is the Baseline one admitted run record stands as.
func baselineOf(candidate *run.Record) Baseline {
	resolved := Baseline{id: candidate.ID, passed: candidate.Passed}
	if candidate.TestsFailed != nil {
		resolved.failed, resolved.counted = *candidate.TestsFailed, true
	}
	return resolved
}

// Resolve returns the run record standing as this baseline at head, and reports
// whether one is there.
//
// "When more than one matches, the most recent MUST be used" (§5.2.6), and the
// most recent is the last match in the file: runs.ndjson is appended to and
// never rewritten (§2.3), and run.NextID allocates above every id the file
// holds, so file order, id order and chronological order are the same order.
// Iterating backwards stops at the first match, which is that one.
func (s Spec) Resolve(stored []run.Record, head string) (Baseline, bool) {
	for i := len(stored) - 1; i >= 0; i-- {
		if candidate := &stored[i]; s.stands(candidate, head) {
			return baselineOf(candidate), true
		}
	}
	return Baseline{}, false
}

// ResolveBaseline returns the run record §5.5's `baseline` column has this
// probe point at, as the Baseline §5.3.5 and §5.4.4 read §5.2.5's verdict off,
// and reports whether that record is there.
//
// It is by id rather than by re-resolution, and the two are not the same
// question. Resolve answers "which run stands as this baseline now", which is
// what a probe about to run needs; §5.4.4 asks after "the probe's `baseline`
// run record", which is the run this experiment was actually measured against.
// A later `cr test` at the same head would change the first answer and must not
// change the second, or a probe would be graded against a run that happened
// after it.
//
// §5.2.6's fence is applied to the named record all the same. The id is read
// out of a record cr wrote, so it cannot have been chosen by an agent — but a
// run carrying a `probe`, or one §5.1.6 found the sandbox unclean after, is not
// a baseline whatever names it, and stands is the one place that says so.
func (r *Record) ResolveBaseline(stored []run.Record) (Baseline, bool) {
	spec := Referenced(r.Kind, r.Filter)
	for i := range stored {
		if candidate := &stored[i]; candidate.ID == r.Baseline &&
			spec.stands(candidate, r.Head) {
			return baselineOf(candidate), true
		}
	}
	return Baseline{}, false
}

// Performer runs one baseline of §5.2.2 and returns the run record that was
// stored for it.
//
// The act is the caller's rather than this package's because performing a
// baseline is running the profile's suite in the sandbox under §5.6.1's lock
// and appending the result under §2.3.1's — all of which `cr test` already
// does, and none of which a rule about which baselines exist has any business
// doing a second time.
type Performer func(Spec) (run.Record, error)

// Ensure returns the baseline §5.5 has a probe of this kind and filter point
// at, performing and recording first every baseline §5.2.2 requires that head
// has no run for yet (§5.2.6).
//
// The performing comes before the probe and never after it, which is what keeps
// the baseline un-probed: the runs it makes are runs of the sandbox as §5.1
// prepared it, so the record §5.2.6 stores is admissible by construction rather
// than by inspection. This is also why a first probe at a fresh head obtains a
// baseline at all — nothing is on file at that head, and what §5.2.6 performs is
// on code no probe has touched.
func Ensure(
	stored []run.Record, head string, kind Kind, filter string, perform Performer,
) (Baseline, error) {
	for _, spec := range Missing(stored, head, Required(kind, filter)) {
		performed, err := perform(spec)
		if err != nil {
			return Baseline{}, err
		}
		stored = append(stored, performed)
	}
	resolved, ok := Referenced(kind, filter).Resolve(stored, head)
	if !ok {
		// The baseline was performed and still does not stand, which
		// means the run that came back is not one §5.2.6 admits: it
		// carries a probe, or §5.1.6's check failed after it. Either
		// way the probe has no foundation and is not run, because a
		// probe graded against a baseline that is not there is exactly
		// the pre-existing failure §5.2.2 exists to keep off a
		// colleague's pull request.
		return Baseline{}, &NoBaselineError{Head: head, Kind: kind}
	}
	return resolved, nil
}

// NoBaselineError reports a baseline Ensure performed that still does not stand.
//
// It names no next step, because the step is about the sandbox the run left
// behind and this package does not know which sandbox that is; the caller that
// ran it does, and says so.
type NoBaselineError struct {
	// Head is the head the baseline was performed at.
	Head string
	// Kind is the kind of probe it was performed for.
	Kind Kind
}

func (e *NoBaselineError) Error() string {
	return fmt.Sprintf("no baseline run stands at %s for a %s probe: "+
		"the run performed for it was not one §5.2.6 admits", e.Head, e.Kind)
}
