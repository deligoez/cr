package probe

// The results §5.3.4's ladder produces, beside ResultError which §5.1.7 already
// declares.
//
// They are unexported on purpose. §5.5 fixes a closed vocabulary per kind and
// says it "MUST NOT be shared", and closing that set — and refusing a record
// carrying a value outside its kind's — is one statement that belongs in one
// place. What is here is only what §5.3.4's rungs name, spelled once so the
// ladder reads as the section reads.
const (
	resultTimeout         Result = "timeout"
	resultNoTestsSelected Result = "no-tests-selected"
	resultInconclusive    Result = "inconclusive"
	resultNoTestFailed    Result = "no-test-failed"
	resultFailed          Result = "failed"
)

// Measured is what one mutation probe run produced, as §5.3.4 needs to read it.
//
// It is a value rather than a set of arguments because the ladder is declared
// "total over every run — no run can match two rungs, and none can match none",
// and totality is a property of the whole input. A caller that passed four
// booleans in some order could leave one out; a caller that fills this in
// cannot, and the zero value is a run whose patch never applied — which rung 1
// answers rather than falling off the end.
type Measured struct {
	// Applied says the mutation patch applied cleanly. When it is false
	// no tests were run, which is rung 1.
	Applied bool
	// TimedOut says the run was killed for exceeding
	// `tests.timeout_seconds` (§5.2.3), which is rung 2.
	TimedOut bool
	// Unstarted says the runner could not be started at all — the
	// program named by `tests.cmd` is not there, or is not executable.
	Unstarted bool
	// ExitCode is the runner's own exit status. A negative code is a
	// process that exited on a signal rather than by returning, which
	// rung 3 puts beside a runner that never started.
	ExitCode int
	// TestsRun is the executed test count, nil when undetermined.
	TestsRun *int
	// TestsFailed is the failed test count, nil when undetermined.
	TestsFailed *int
}

// Ladder is §5.3.4: the result of a mutation probe, by the first matching rung.
//
// The order is the section's own and is not an implementation choice. `timeout`
// sits above `error` because a run that never finished said nothing about the
// code while one that died on a signal did not run at all; both sit above every
// reading of the counts, because a killed or unstarted runner's counts are
// whatever it managed to print before it stopped. And rung 4 sits above rung 5
// so that a runner which selected nothing and said so is `no-tests-selected`
// rather than `inconclusive` — §5.3.5 refuses a `probed` grade to both, and the
// reader is owed the difference.
//
// The result this returns is the ladder's, not the record's. §5.1.7 sits above
// it and Decide is what applies that, so nothing here has to know about the
// sandbox: a value produced here reaches probes.ndjson only through Decide.
func Ladder(m Measured) Result {
	switch {
	case !m.Applied:
		return ResultError
	case m.TimedOut:
		return resultTimeout
	case m.Unstarted || m.ExitCode < 0:
		return ResultError
	case m.TestsRun != nil && *m.TestsRun == 0:
		return resultNoTestsSelected
	case m.TestsRun == nil || m.TestsFailed == nil:
		return resultInconclusive
	case *m.TestsFailed == 0:
		return resultNoTestFailed
	}
	return resultFailed
}

// Proves is §5.3.5: this probe proves the gap, and so may be what a record
// graded `probed` rests on under §6.2.
//
// Two conditions and no others. The result has to be `no-test-failed` — the one
// rung of §5.3.4's ladder on which the suite ran, selected tests, and noticed
// nothing — and the baseline run has to have passed per §5.2.5, because a suite
// already red at this head makes every mutation look survivable.
//
// The two arguments are what makes the refusal structural rather than
// remembered. An Outcome exists only where Decide made one, so §5.1.7 has
// already had its say and a voided probe arrives carrying `error`; a Baseline
// exists only where Spec.Resolve built one out of a run record §5.2.6 admits,
// so the verdict read here is §5.2.5's on the record the resolution actually
// chose rather than on some other run the caller had in hand. Neither can be
// assembled out of a value a caller preferred, so `timeout`, `error`,
// `no-tests-selected` and `inconclusive` have no path to a `probed` grade at
// all: an empty selection, an unparseable runner and a run that never finished
// cannot manufacture evidence. A record resting on one of them is left `argued`
// by §6.2 and posted as a question by §6.3.
//
// §5.4.4's conditions are a different question about a different ladder and
// belong with the gap probe, which is why this reads the mutation vocabulary
// alone.
func Proves(outcome Outcome, baseline Baseline) bool {
	return outcome.Result() == resultNoTestFailed && baseline.Passed()
}

// Disproves is §5.3.7: a test caught the mutation, so the gap is not there and
// the agent does not raise the finding.
//
// It asks nothing of the baseline, and that is §5.3.7's own shape rather than
// an omission here. §5.3.5's baseline condition guards an assertion cr would
// otherwise make to a colleague; there is no assertion on this side, only a
// finding cr declines to raise, and a suite that noticed the mutation noticed
// it whatever else at this head is failing.
//
// A voided probe never reaches it. §5.1.7 says such a probe "establishes
// nothing in either direction", and the outcome Decide handed back carries
// `error` rather than the `failed` this keys on — so the suppression is not
// read off a run whose sandbox had drifted under it.
func Disproves(outcome Outcome) bool {
	return outcome.Result() == resultFailed
}
