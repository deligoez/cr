package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §5.5's second table, transcribed from the spec in the order it lists each
// row, so the vocabularies cr closes are checked against the section rather
// than against themselves.
var specVocabularies = map[Kind][]Result{
	Mutation: {"no-test-failed", "failed", "no-tests-selected", "inconclusive", "error", "timeout"},
	Gap:      {"passed", "failed", "no-tests-selected", "inconclusive", "error", "timeout"},
}

// §5.5: "the result vocabulary is per kind and MUST NOT be shared".
//
// The values are written out as literal strings rather than through the
// package's own constants, because the constants are what the section is meant
// to be checking: a renamed value that both the ladder and the vocabulary
// followed would change what reaches probes.ndjson with nothing noticing.
func TestTheResultVocabularyIsTheOneTheSpecWritesPerKind(t *testing.T) {
	assert.Equal(t, specVocabularies[Mutation], Results(Mutation))
	assert.Equal(t, specVocabularies[Gap], Results(Gap))
	assert.Empty(t, Results("coverage"), "§5.5 names two kinds of probe and no third")
}

// The vocabulary a caller is handed is a copy, so a caller cannot widen the set
// a stored record is held to.
func TestAKindsVocabularyCannotBeWidenedByItsReader(t *testing.T) {
	held := Results(Mutation)
	held[0] = "passed"
	assert.Equal(t, specVocabularies[Mutation], Results(Mutation))
}

// §5.5 at the record boundary: a record carrying a result outside its kind's
// vocabulary is rejected.
//
// The two crossed values are the whole reason the sets are per kind. `passed`
// on a mutation probe would read as a suite that noticed nothing, which is the
// one result §5.3.5 lets a `probed` grade rest on; `no-test-failed` on a gap
// probe would read as the missing test §5.4.5 says in as many words a gap probe
// never establishes. Either one crossing over turns a question into an
// assertion, so both are asserted refused.
func TestAResultOutsideItsKindsVocabularyIsRejected(t *testing.T) {
	for name, tc := range map[string]struct {
		kind   Kind
		result Result
	}{
		"passed is not a mutation result":        {kind: Mutation, result: "passed"},
		"no-test-failed is not a gap result":     {kind: Gap, result: "no-test-failed"},
		"a value neither kind names":             {kind: Mutation, result: "survived"},
		"a value neither kind names, on a gap":   {kind: Gap, result: "survived"},
		"the empty result is not a result":       {kind: Mutation, result: ""},
		"a kind §5.5 does not name carries none": {kind: "coverage", result: "failed"},
		"an empty kind carries none":             {kind: "", result: "error"},
		"case is not close enough":               {kind: Mutation, result: "Failed"},
		"nor is the other row's spelling padded": {kind: Gap, result: " passed"},
	} {
		t.Run(name, func(t *testing.T) {
			var refused *OutsideVocabularyError
			require.ErrorAs(t,
				CheckResult(&Record{Kind: tc.kind, Result: tc.result}), &refused)
			assert.Equal(t, tc.kind, refused.Kind)
			assert.Equal(t, tc.result, refused.Result)
			assert.Contains(t, refused.Error(), "§5.5")
		})
	}
}

// Every value the section admits is admitted, which is the half a check that
// refused everything would also pass.
func TestEveryResultItsKindAdmitsIsAccepted(t *testing.T) {
	for kind, admitted := range specVocabularies {
		for _, result := range admitted {
			t.Run(string(kind)+"/"+string(result), func(t *testing.T) {
				assert.NoError(t, CheckResult(&Record{Kind: kind, Result: result}))
			})
		}
	}
}

// A refusal names what the kind may carry, because the mistake it catches is
// almost always a value borrowed from the other row — and a reader told only
// that the value is wrong has to open the spec to see which row it came from.
func TestARefusalNamesWhatTheKindMayCarry(t *testing.T) {
	refused := CheckResult(&Record{Kind: Gap, Result: "no-test-failed"})
	require.Error(t, refused)
	assert.Contains(t, refused.Error(), "passed, failed, no-tests-selected, inconclusive, error, timeout")

	unknown := CheckResult(&Record{Kind: "coverage", Result: "failed"})
	require.Error(t, unknown)
	assert.Contains(t, unknown.Error(), "names two kinds of probe",
		"a kind with no vocabulary is told there is no third kind, not shown an empty set")
}
