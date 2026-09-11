package draft

import (
	"strings"
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

// The entry is the region as the file holds it, so a field ending on newlines
// is recorded without them: in draft.md those newlines are indistinguishable
// from the blank line that separates the body from what follows, and reading
// the untouched block back recovers the body without them. An entry keeping
// them would differ from every untouched body, and §7.1.6 would take a block
// nobody edited for an edited one.
func TestAnEntryIsWhatReadingTheUntouchedBlockBackRecovers(t *testing.T) {
	trailing := aRecord("f1")
	trailing.Evidence += "\n\n"

	rendered, err := Rendered([]*finding.Finding{trailing}, render.LangEN, nil)
	require.NoError(t, err)

	assert.Equal(t, trailing.Summary+"\n\n"+strings.TrimRight(trailing.Evidence, "\n"), rendered["f1"])
}

// A rendered.json cr cannot read is refused naming the file, rather than read
// as empty. Empty would keep every body, so nothing would be lost, but it would
// be a silent answer about a file cr wrote itself.
func TestAnUnreadableRenderedJSONIsRefusedNamingTheFile(t *testing.T) {
	entries, err := DecodeRendered("rendered.json", []byte(`{"f1": 7}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rendered.json")
	assert.Nil(t, entries)
}
