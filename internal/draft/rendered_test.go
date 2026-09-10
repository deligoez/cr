package draft

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// §7.1.5: each record's agent region exactly as rendered, and only that — the
// cr-owned regions are excluded, since cr regenerates them.
//
// The record carries both owned regions the draft renders today: it is a
// question, so §8.1.4's label opens it, and its suggestion is the rule's, so
// §8.1.6's provenance region follows. The entry must hold neither, and must be
// the body exactly: the summary and the evidence as §8.1.2 places them, and the
// labelled suggestion block §7.1.3 puts inside the agent's own region.
func TestAnEntryIsTheAgentRegionWithoutTheOwnedRegions(t *testing.T) {
	question := aRecord("f1")
	question.Kind, question.Grade = finding.KindQuestion, finding.GradeArgued
	question.Summary = "Does the caller ever see the error Decode returns?"
	question.Suggestion, question.SuggestionOrigin = "if err != nil {\n\treturn err\n}", finding.OriginRule

	rendered, err := Rendered([]*finding.Finding{question}, render.LangEN, nil)
	require.NoError(t, err)

	require.Contains(t, rendered, "f1")
	assert.Equal(t, body(question), rendered["f1"], "the entry is the agent region, byte for byte")
	assert.NotContains(t, rendered["f1"], render.Reserved, "no cr-owned region reaches the entry")
	assert.Contains(t, renderOf(t, question), rendered["f1"],
		"and it is the region as the draft carries it, not a second rendering of it")
}
