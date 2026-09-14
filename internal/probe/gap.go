package probe

// The result §5.4.3's ladder produces that §5.3.4's does not. The rest of its
// rungs answer with values mutation.go already spells, and they are spelled
// once for the reason they are spelled at all: §5.5 fixes a closed vocabulary
// per kind and says it "MUST NOT be shared", so the set a kind may carry is one
// statement, made once in vocabulary.go, and what is here is only what the
// rungs name.
const resultPassed Result = "passed"

// GapMeasured is what one gap probe run produced, as §5.4.3 needs to read it.
//
// It is a value rather than a set of arguments for the reason Measured is: the
// ladder is declared "total over every run", and totality is a property of the
// whole input. It is a separate type from Measured rather than a reuse of it
// because §5.4.3's ladder has no rung for a patch that did not apply — a gap
// probe applies no patch — and a field the ladder never reads would be a field
// a caller could fill in and be answered nothing about.
type GapMeasured struct {
	// TimedOut says the run was killed for exceeding
	// `tests.timeout_seconds` (§5.2.3), which is rung 1.
	TimedOut bool
	// Unstarted says the runner could not be started at all — the
	// program named by `tests.cmd` is not there, or is not executable —
	// which is rung 2.
	Unstarted bool
	// ExitCode is the runner's own exit status. A negative code is a
	// process that exited on a signal rather than by returning, which
	// rung 2 puts beside a runner that never started.
	ExitCode int
	// TestsRun is the executed test count, nil when undetermined.
	TestsRun *int
	// TestsFailed is the failed test count, nil when undetermined.
	TestsFailed *int
	// Detail is what rung 2 read, for GapReason to name: what the attempt
	// to start the runner reported, or the signal the runner exited on.
	Detail string
}

// GapLadder is §5.4.3: the result of a gap probe, by the first matching rung.
//
// The order is the section's own and is not an implementation choice. It is
// §5.3.4's order without its first rung, because §5.4.2 places a file rather
// than applying a patch, and with its last two rungs reading the opposite way:
// where a mutation that nothing noticed is `no-test-failed`, a supplied test
// that nothing failed is `passed`, and §5.4.5 is explicit that the two are not
// the same fact.
//
// Rung 4 exists for the same reason as §5.3.4's fifth: without it a profile
// that cannot report counts would drive every gap probe to `failed`, and §5.4.4
// would raise it at severity `high` on no evidence at all.
//
// Rung 5 answers `passed` only for a run that exited 0, and `inconclusive`
// otherwise, as §5.3.4's rung 6 does: a runner that printed its count and then
// exited non-zero has not said that nothing failed.
//
// Rung 2 answers a runner that exited on a signal as well as one that never
// started, which is round 9's probe-ladder-asymmetry finding. Without it a
// runner killed by the out-of-memory killer, or one that segmentation-faulted
// after printing output `tests.count_pattern` still matches, falls through to
// `failed` — and §5.4.4 makes a supported failed gap probe carry severity at
// least `high`, so a crashed process would become the loudest item in the
// draft. The reading is §5.3.4's rung 3 exactly, and the two ladders are the
// same shape for the same reason.
//
// The result this returns is the ladder's, not the record's. §5.1.7 sits above
// it and Decide is what applies that, so nothing here has to know about the
// sandbox: a value produced here reaches probes.ndjson only through Decide.
func GapLadder(m GapMeasured) Result {
	switch {
	case m.TimedOut:
		return resultTimeout
	case m.Unstarted || m.ExitCode < 0:
		return ResultError
	case m.TestsRun != nil && *m.TestsRun == 0:
		return resultNoTestsSelected
	case m.TestsRun == nil || m.TestsFailed == nil:
		return resultInconclusive
	case *m.TestsFailed == 0:
		if m.ExitCode != 0 {
			return resultInconclusive
		}
		return resultPassed
	}
	return resultFailed
}

// GapReason says why GapLadder answered `error`, and is empty for every other
// rung, as Reason does for §5.3.4's ladder.
func GapReason(m GapMeasured) string {
	switch {
	case m.TimedOut:
		return ""
	case m.Unstarted:
		return "§5.4.3's second rung: " + notStarted(m.Detail)
	case m.ExitCode < 0:
		return "§5.4.3's second rung: " + exitedOnSignal(m.Detail)
	}
	return ""
}
