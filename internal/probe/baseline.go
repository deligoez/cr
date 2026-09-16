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
	"slices"

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
// un-probed code, narrowed to Filter and to Paths.
//
// It is a value rather than an id because a baseline is identified by what it
// measured before it is identified by which record holds it. §5.2.2 requires
// the measurement "once per head, sandbox generation, filter, and paths", and
// whether a record for it already exists is a question asked of runs.ndjson
// with this value in hand — the generation by the runs OfSandbox narrows to,
// the head by Resolve, and the other two by the fields here.
type Spec struct {
	// Filter is the expression the baseline run must have been narrowed
	// to, and empty for a run of every test the paths select.
	Filter string
	// Paths are the `--path` values it must have been narrowed to, in
	// the order they were given, and empty for a run of the whole suite.
	Paths []string
}

// Referenced is the baseline §5.5's `baseline` column has a probe of this kind
// point at: for `mutation`, the run carrying the probe's own filter and paths;
// for `gap`, the run carrying its paths and no filter, which is the whole suite
// when it has none.
//
// The filter is read for one kind and ignored for the other, and that asymmetry
// is the round 8 repair rather than an oversight. A gap probe's test does not
// exist in the baseline — §5.4.2 places it in the sandbox for the probe run and
// removes it again — so a baseline narrowed to the probe's filter would select
// a file that is not there. Its `tests_run` would be zero, §5.2.5 would
// therefore never call it passed, and §5.2.6 would re-run it before every probe
// and never resolve one. The obligation is scoped to mutation probes, where the
// filter selects tests that exist on both sides of the mutation.
//
// The paths are read for both, because §5.4.2 makes a gap probe's `--path`
// values the population its baseline measures: the probe's own run is narrowed
// to the file it placed, and the question the baseline answers is whether the
// tests already in those paths were passing.
func Referenced(kind Kind, filter string, paths []string) Spec {
	if kind == Mutation {
		return Spec{Filter: filter, Paths: paths}
	}
	return Spec{Paths: paths}
}

// Required is every baseline §5.2.2 has recorded at the head and the sandbox
// generation before a probe of this kind runs.
//
// There is exactly one, and that is §5.2.2's own sentence: a probe's baseline
// is "a run on unmutated, un-probed code of the tests the probe runs". The
// whole suite is that run only when the probe runs the whole suite. Recording
// an unfiltered run beside a filtered probe's baseline would measure a
// population no rung of §5.3.4 or §5.4.3 reads, and would spend the suite's
// whole runtime doing it.
func Required(kind Kind, filter string, paths []string) []Spec {
	return []Spec{Referenced(kind, filter, paths)}
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
		if _, resolved := spec.Resolve(stored, head); !resolved {
			missing = append(missing, spec)
		}
	}
	return missing
}

// OfSandbox returns the runs of stored measured in the sandbox generation
// named, in file order, for Missing and Ensure to resolve baselines among.
//
// A run of an earlier generation measured a sandbox §5.1.6 has since rebuilt,
// because its tracked files, its copied files or its head no longer stood, so
// it is not a run of "the same tests on unmutated, un-probed code" (§5.5) as the
// sandbox a probe now runs in, and §5.2.6 performs the baseline again.
//
// §5.2.6's last sentence is the empty generation: "A run record carrying no
// `sandbox` matches no sandbox generation." A run written before generations
// were recorded names none, and it is refused here rather than left to the
// caller — sandbox.Ensure admits no sandbox whose post-setup baseline names no
// generation (§5.1.6), so the two cannot meet today, and a rule that holds only
// because of what a caller happens to pass is a rule one edit away from not
// holding. Refused on both sides: a generation of "" matches nothing, and a
// record carrying none matches nothing.
func OfSandbox(stored []run.Record, generation string) []run.Record {
	measured := make([]run.Record, 0, len(stored))
	if generation == "" {
		return measured
	}
	for i := range stored {
		if stored[i].Sandbox == generation {
			measured = append(measured, stored[i])
		}
	}
	return measured
}

// stands reports whether one run record is a baseline answering this spec.
//
// It is the one candidate predicate: Missing asks whether §5.2.2's "once per
// head, sandbox generation, filter, and paths" is already satisfied and Resolve
// asks which record §5.5 points at, and a second copy of these conditions would
// be a second answer that could drift from the first. The generation is not
// among them because OfSandbox has already narrowed the records to it.
//
// The last two conditions are §5.2.6's fence: only a run carrying no `probe`
// may serve as a baseline, so a probe's own mutated or probe-injected
// run cannot become the next probe's baseline. Passing is deliberately not a
// condition here — §5.2.5's verdict is what §5.3.5 and §5.4.4 read off the
// resolved baseline, and a failing baseline is the signal that the repository
// has pre-existing failures rather than a baseline that does not exist. Making
// it a resolution condition would have cr re-run a red suite before every
// probe, for ever, and never resolve one.
func (s Spec) stands(candidate *run.Record, head string) bool {
	return candidate.Head == head &&
		candidate.Filter == s.Filter &&
		slices.Equal(candidate.Paths, s.Paths) &&
		candidate.Probe == "" &&
		!candidate.Contaminated
}
