package probe

// Result is one value of §5.5's `result` column.
//
// The per-kind vocabularies of §5.5's second table are not declared here, and
// deliberately: §5.5 says the vocabulary "is per kind and MUST NOT be shared",
// so which values a `mutation` may carry and which a `gap` may carry is one
// statement, made once in vocabulary.go. What is here is the one value §5.1.7
// reaches for, which both vocabularies already name.
type Result string

// ResultError is §5.1.7's override.
//
// It is not a rung of §5.3.4's or §5.4.3's ladder. Both ladders are declared
// total over every run — "no run can match two rungs, and none can match none"
// — and a rung for an unclean sandbox would make that claim false, since the
// cleanliness check reads the sandbox rather than the run. §5.1.7 instead sits
// above the ladder and replaces whatever it produced.
const ResultError Result = "error"

// Outcome is what one probe run records: the ladder's answer, after §5.1.7's
// override has had its say.
//
// Its fields are unexported and Decide is its only constructor, and that is
// round 9's probe-result-override finding made structural. §5.1.7 fixes the
// order — "the post-run cleanliness check runs before the probe record is
// written" — and without that order §5.3.4's and §5.4.3's totality claims
// collide with §5.5.1's immutability: it would be undecidable whether `error`
// is the only value ever persisted or whether an already-written record has to
// be amended into one. A result that can only be reached through Decide is a
// result the check has already had its say about, so exactly one value is ever
// written and §5.5.1 never has to be broken to correct it.
type Outcome struct {
	// result is the value the probe record carries.
	result Result
	// voided says §5.1.6's check failed after the run.
	voided bool
	// unclean is what that check found, and empty when it passed.
	unclean string
}

// Decide applies §5.1.7 to a ladder outcome, given what §5.1.6's post-run check
// found. An empty unclean is a sandbox that passed the check.
//
// The ladder value is taken and discarded rather than consulted, whatever it
// was. §5.1.7 says such a probe "establishes nothing in either direction", and
// that cuts both ways: a mutation probe's `no-test-failed` cannot prove the gap
// §5.3.5 would let it prove, and its `failed` cannot disprove one either — so
// §5.3.7's suppression is not read from it, because the value §5.3.7 keys on is
// not the value that was written.
//
// §5.4.5's severity ceiling needs no special case for the same reason in
// reverse: it already names `error` among the results that may not support a
// `probed` grade and caps severity at `medium`, so a voided gap probe reaches
// it as a value it was already written to handle.
func Decide(ladder Result, unclean string) Outcome {
	if unclean == "" {
		return Outcome{result: ladder}
	}
	return Outcome{result: ResultError, voided: true, unclean: unclean}
}

// Result is the value §5.5's `result` column carries, which is the ladder's
// unless §5.1.7 overrode it.
func (o Outcome) Result() Result { return o.result }

// Voided reports that §5.1.6's check failed after the run, which is the one
// fact §5.1.7's three consequences all follow from: the record carries `error`,
// the probe grades no finding, and the sandbox is recreated before the next
// run.
func (o Outcome) Voided() bool { return o.voided }

// Reason is why the record carries `error` or `inconclusive`, given ladder, the
// ladder's own reason from Reason or GapReason. A probe §5.1.7 voided carries
// what the post-run check found instead, because its `error` is not the
// ladder's; any other result carries none.
func (o Outcome) Reason(ladder string) string {
	switch {
	case o.voided:
		return "§5.1.7: the post-run cleanliness check failed, so the result is error whatever " +
			"the ladder read, the probe grades no finding, and the sandbox is recreated before " +
			"the next run: " + o.unclean
	case o.result != ResultError && o.result != resultInconclusive:
		return ""
	}
	return ladder
}
