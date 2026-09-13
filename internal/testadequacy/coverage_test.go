package testadequacy

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.1.3 lists "whether a unit is covered by tests" among the judgements that
// belong to the agent alone, and §4.4.1 gives cr the other half of that
// sentence: it attaches the evidence and records the classification. The rule is
// easy to state and easy to break by helpfulness — a default of `uncovered` when
// no test file was attached would look like a courtesy and would be cr forming
// the opinion P5 forbids.
//
// So the assertion is structural rather than behavioural. Every field of
// Coverage is unexported and no exported function takes a Classification, which
// leaves UnmarshalJSON — the agent's own line — as the only way a classification
// gets into one. A future change that exports the field to make some call site
// convenient fails here, which is the point: this test guards the shape, not a
// particular caller's restraint.
func TestNothingInCrCanFillInAClassification(t *testing.T) {
	value := reflect.TypeFor[Coverage]()
	require.Positive(t, value.NumField())
	for field := range value.Fields() {
		assert.False(t, field.IsExported(), "Coverage.%s is exported, so cr can classify a unit", field.Name)
	}

	// The zero value is the honest one: no verdict, and no paths to have
	// rested on. It is what a cell holds before the agent has spoken.
	var unclassified Coverage
	assert.Empty(t, unclassified.Classification())
	assert.NotNil(t, unclassified.TestPaths())
	assert.Empty(t, unclassified.TestPaths())
}

// §4.4.1 requires the classification to be recorded in the coverage cell
// "together with the test paths it rested on", so the two travel as one value:
// a verdict stored without the evidence behind it cannot be re-read later, and
// §9 re-reads every round's cells.
//
// The round trip is the assertion because the cell is stored as NDJSON. A value
// that decodes but does not encode back to the same thing is a cell that changes
// between the round that filled it and the round that reads it.
func TestACoverageValueRoundTripsTheAgentsDecision(t *testing.T) {
	line := []byte(`{"classification":"partially-covered","test_paths":["tests/Feature/OrderTest.php"]}`)

	var recorded Coverage
	require.NoError(t, json.Unmarshal(line, &recorded))
	assert.Equal(t, PartiallyCovered, recorded.Classification())
	assert.Equal(t, []string{"tests/Feature/OrderTest.php"}, recorded.TestPaths())

	encoded, err := json.Marshal(recorded)
	require.NoError(t, err)
	assert.JSONEq(t, string(line), string(encoded))

	t.Run("a verdict that rested on no test file keeps an empty array", func(t *testing.T) {
		var uncovered Coverage
		require.NoError(t, json.Unmarshal([]byte(`{"classification":"uncovered"}`), &uncovered))

		encoded, err := json.Marshal(uncovered)
		require.NoError(t, err)
		// A slice reaching JSON as null is the convention this
		// repository keeps, and a reader distinguishing null from []
		// would read "cr does not know which tests" from a cell that
		// says "none".
		assert.JSONEq(t, `{"classification":"uncovered","test_paths":[]}`, string(encoded))
	})

	t.Run("the third verdict", func(t *testing.T) {
		var covered Coverage
		require.NoError(t, json.Unmarshal([]byte(`{"classification":"covered","test_paths":[]}`), &covered))
		assert.Equal(t, Covered, covered.Classification())
	})
}

// §4.4.1 names exactly three classifications, and a value outside them is
// refused rather than carried.
//
// A cell is what §10.2 reads completeness out of, so a verdict nothing
// downstream recognises would let a round report itself complete on a word no
// rule in the spec defines — the failure a written-and-never-called validator
// produces, with every prompt still promising that unknown values are rejected.
// The empty string is one of those values, so a coverage object that omits the
// classification is refused on the same path rather than defaulting to anything.
func TestAnUnrecognisedClassificationIsRejected(t *testing.T) {
	for _, line := range []string{
		`{"classification":"mostly-covered","test_paths":[]}`,
		`{"classification":"","test_paths":[]}`,
		`{"test_paths":[]}`,
		// The vocabulary is the three words as §4.4.1 writes them, and
		// a spelling that only reads the same is a fourth value.
		`{"classification":"Covered","test_paths":[]}`,
		`{"classification":"partially covered","test_paths":[]}`,
	} {
		var refused Coverage
		err := json.Unmarshal([]byte(line), &refused)

		require.Error(t, err, "%s was accepted", line)
		var invalid *InvalidClassificationError
		require.ErrorAs(t, err, &invalid)
		// §12.4: the message names the next actionable step, which is
		// the set the agent may write.
		assert.Contains(t, invalid.Error(), "partially-covered")
		assert.Empty(t, refused.Classification(), "a refused line left a verdict behind")
	}
}
