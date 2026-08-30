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
	value := reflect.TypeOf(Coverage{})
	require.Positive(t, value.NumField())
	for i := range value.NumField() {
		field := value.Field(i)
		assert.False(t, field.IsExported(), "Coverage.%s is exported, so cr can classify a unit", field.Name)
	}

	// The zero value is the honest one: no verdict, and no paths to have
	// rested on. It is what a cell holds before the agent has spoken.
	var unclassified Coverage
	assert.Empty(t, unclassified.Classification())
	assert.NotNil(t, unclassified.TestPaths())
	assert.Empty(t, unclassified.TestPaths())
}

