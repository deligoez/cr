// Package probe holds §5's probe machinery: the baselines §5.2.2 requires
// recorded before a probe is graded, and the ladders §5.3 and §5.4 read.
//
// The baseline is the whole reason a probe can assert anything. A mutation
// probe's claim is "the suite did not notice this break", and that claim is
// only worth making when the suite noticed nothing else either — a repository
// with a red suite makes every mutation look survivable. §5.2.2 answers by
// requiring a run on un-probed code at the same head, and §5.3.5 and §5.4.4
// then refuse a `probed` grade unless that run passed.
package probe

import (
	"github.com/deligoez/cr/internal/run"
)

// Kind is §5.5's `kind` column: the two sorts of probe §5.3 and §5.4 define.
//
// It lives here rather than beside the ladders because the baseline rule is the
// first thing that reads it — §5.2.2 and §5.5's `baseline` row give the two
// kinds different baselines, and that difference is what Referenced encodes.
type Kind string

const (
	// Mutation is §5.3's probe: production code is broken and the suite
	// is expected to notice.
	Mutation Kind = "mutation"
	// Gap is §5.4's probe: a new test is supplied and run.
	Gap Kind = "gap"
)

// Spec names one baseline run: a run of the suite at the current head, on
// un-probed code, narrowed to Filter.
//
// It is a value rather than an id because a baseline is identified by what it
// measured before it is identified by which record holds it. §5.2.2 requires
// the measurement "once per head", and whether a record for it already exists
// is a question asked of runs.ndjson with this value in hand.
type Spec struct {
	// Filter is the expression the baseline run must have been narrowed
	// to, and empty for the unfiltered run of §5.2.2's first sentence.
	Filter string
}

// unfiltered is §5.2.2's first sentence: the whole suite, no filter.
var unfiltered = Spec{}

// Referenced is the baseline §5.5's `baseline` column has a probe of this kind
// point at: for `mutation`, the run carrying the probe's own filter; for `gap`,
// the unfiltered run.
//
// The filter argument is read for one kind and ignored for the other, and that
// asymmetry is the round 8 repair rather than an oversight. A gap probe's test
// does not exist in the baseline — §5.4.2 places it in the sandbox for the
// probe run and removes it again — so a baseline narrowed to the probe's filter
// would select a file that is not there. Its `tests_run` would be zero, §5.2.5
// would therefore never call it passed, and §5.2.6 would re-run it before every
// probe and never resolve one. The obligation is scoped to mutation probes,
// where the filter selects tests that exist on both sides of the mutation.
func Referenced(kind Kind, filter string) Spec {
	if kind == Mutation {
		return Spec{Filter: filter}
	}
	return unfiltered
}

// Required is every baseline §5.2.2 has recorded at the head before a probe of
// this kind runs, in the order they are recorded and with Referenced last.
//
// The unfiltered run is always among them, whatever the kind and whatever the
// filter: §5.2.2's first sentence is unconditional, because a filtered run
// narrowed to the mutated code says nothing about the failures the rest of the
// suite is already carrying. The filtered run of the second sentence joins it
// only for a filtered mutation probe, and only as an addition — "additionally"
// is the word the section uses.
//
// An unfiltered mutation probe needs one baseline, not two: Referenced is then
// the unfiltered run itself, and recording it twice would measure the same
// thing under two ids.
func Required(kind Kind, filter string) []Spec {
	required := []Spec{unfiltered}
	if referenced := Referenced(kind, filter); referenced != unfiltered {
		required = append(required, referenced)
	}
	return required
}

// Missing returns the baselines of required that the pull request has no run
// record for at head, in the order Required gave them.
//
// This is §5.2.2's "once per head" in the only form it can honestly take. There
// is no memo file: runs.ndjson already stamps `head` on every record it holds
// (§2.3.3), so the question "has this baseline been recorded at this head" is
// answered from the runs themselves. A moved head invalidates every baseline
// for free, because no record at the old head matches the new one — and it has
// to, since §5.5.3 keeps records from earlier heads on disk. A separate memo
// would be a second answer that could disagree with the file it summarises.
func Missing(stored []run.Record, head string, required []Spec) []Spec {
	missing := make([]Spec, 0, len(required))
	for _, spec := range required {
		if !spec.recorded(stored, head) {
			missing = append(missing, spec)
		}
	}
	return missing
}

// recorded reports whether any run record already stands as this baseline.
func (s Spec) recorded(stored []run.Record, head string) bool {
	// Indexed rather than ranged by value: a run record carries an output
	// tail, and only four of its fields are read here.
	for i := range stored {
		if s.stands(&stored[i], head) {
			return true
		}
	}
	return false
}

// stands reports whether one run record is a baseline answering this spec.
//
// Three conditions, and §5.2.6 supplies the third: only a run carrying no
// `probe` may serve as a baseline, so a probe's own mutated or probe-injected
// run cannot become the next probe's baseline. Passing is deliberately not a
// condition here — §5.2.5's verdict is what §5.3.5 and §5.4.4 read off the
// resolved baseline, and a failing baseline is the signal that the repository
// has pre-existing failures rather than a baseline that does not exist. Making
// it a resolution condition would have cr re-run a red suite before every
// probe, for ever, and never resolve one.
func (s Spec) stands(candidate *run.Record, head string) bool {
	return candidate.Head == head &&
		candidate.Filter == s.Filter &&
		candidate.Probe == "" &&
		!candidate.Contaminated
}
