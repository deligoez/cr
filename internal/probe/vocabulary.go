package probe

import (
	"fmt"
	"slices"
	"strings"
)

// vocabularies is §5.5's second table: the results each kind of probe may
// carry, in the order the section lists them.
//
// This is the one place the sets are closed, which is what §5.5's "the result
// vocabulary is per kind and MUST NOT be shared" asks for. The two ladders
// spell only the values their own rungs produce; whether a stored record may
// carry a value is a different question, asked of the record rather than of the
// run that produced it, and answering it beside each ladder would be two
// answers that can drift.
//
// Five of the six values appear in both rows, and that is not sharing. The one
// value each row does not hold is the whole point: §5.3.4's `no-test-failed`
// says the suite did not notice a break, §5.4.3's `passed` says a supplied test
// asserts behaviour that is present, and §5.4.5 is explicit that the two are
// not the same fact. A vocabulary that admitted either value for either kind
// would let a gap probe be read as the missing test only §5.3 establishes.
var vocabularies = map[Kind][]Result{
	Mutation: {
		resultNoTestFailed,
		resultFailed,
		resultNoTestsSelected,
		resultInconclusive,
		ResultError,
		resultTimeout,
	},
	Gap: {
		resultPassed,
		resultFailed,
		resultNoTestsSelected,
		resultInconclusive,
		ResultError,
		resultTimeout,
	},
}

// Results is the vocabulary §5.5 gives one kind, in the section's order, and
// empty for a kind §5.5 does not name.
//
// The result is a copy, so a caller can neither widen a vocabulary nor reorder
// one.
func Results(kind Kind) []Result {
	held := vocabularies[kind]
	return append(make([]Result, 0, len(held)), held...)
}

// OutsideVocabularyError reports a probe record whose result its kind's
// vocabulary does not hold.
//
// It names the kind, the value, and what that kind may carry, because the
// mistake this catches is almost always a value borrowed from the other row:
// `passed` on a mutation probe reads as a suite that noticed nothing, and
// `no-test-failed` on a gap probe reads as the missing test §5.4.5 says a gap
// probe never establishes. Printing the admissible set beside the refusal is
// what makes the difference visible rather than something to look up.
type OutsideVocabularyError struct {
	// Kind is the probe's own kind, as the record carries it.
	Kind Kind
	// Result is the value the record would have been written with.
	Result Result
}

func (e *OutsideVocabularyError) Error() string {
	held := Results(e.Kind)
	if len(held) == 0 {
		return fmt.Sprintf(
			"probe kind %q has no result vocabulary: §5.5 names two kinds of probe, "+
				"mutation (§5.3) and gap (§5.4)", e.Kind)
	}
	named := make([]string, 0, len(held))
	for _, result := range held {
		named = append(named, string(result))
	}
	return fmt.Sprintf(
		"result %q is not one a %s probe may carry: §5.5 fixes the vocabulary per kind and "+
			"says it must not be shared, so a %s probe carries one of %s",
		e.Result, e.Kind, e.Kind, strings.Join(named, ", "))
}

// CheckResult holds one probe record to its kind's vocabulary, and is what
// §5.5's "MUST NOT be shared" comes to at the boundary where a record is
// stored.
//
// It is asked of the record rather than of the ladder that produced the value,
// and that is the difference between this and the two ladders. A ladder can
// only be trusted for the runs that go through it: §5.1.7's override replaces
// whatever it answered, a record can be assembled from fields rather than from
// an Outcome, and a kind the command did not recognise reaches no ladder at
// all. What a reader of probes.ndjson is promised is that every stored line
// carries a value its kind admits, and only a check at the write can promise
// that.
//
// A kind outside §5.5's two is refused here too, on the same call. Its
// vocabulary is empty, so no value could satisfy it, and a record naming a
// third kind would be a line §5.4.4, §5.4.5 and §5.3.5 all read and none of
// them describes.
func CheckResult(record *Record) error {
	if !slices.Contains(vocabularies[record.Kind], record.Result) {
		return &OutsideVocabularyError{Kind: record.Kind, Result: record.Result}
	}
	return nil
}
