package draft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// bodyOf is the body of the one block Render produced for record: everything
// after the marker line and the blank line beneath it, without the block's
// closing newline.
func bodyOf(t *testing.T, record *finding.Finding) string {
	t.Helper()
	rendered := renderOf(t, record)
	_, body, found := strings.Cut(rendered, " -->\n\n")
	require.True(t, found, "a block is a marker, a blank line, and a body")
	return strings.TrimSuffix(body, "\n")
}

// §8.1.2: cr renders the initial body from the record's `summary` and
// `evidence`, in English, and cr never composes.
//
// The whole body is compared rather than searched, because the second half of
// that sentence is only checkable that way. A body that contained both fields
// and also a sentence of cr's own — a heading, a connective, a translation —
// would pass a Contains and still be cr composing reader-facing prose. Equality
// leaves no room for a word the record does not hold.
func TestTheInitialBodyIsTheSummaryThenTheEvidenceVerbatim(t *testing.T) {
	record := aRecord("f1")

	assert.Equal(t,
		"The error Decode returns is dropped.\n\n"+
			"The call's second result is assigned to the blank identifier.",
		bodyOf(t, record),
		"§8.1.2: the summary, then the evidence, each carried as the record holds it")
}

// The two fields are placed, not reworded: whatever the agent wrote reaches the
// body byte for byte, Markdown and all, since the body is the agent's to
// rewrite in `render.lang` and a tidied copy would be cr's prose.
func TestTheInitialBodyCarriesBothFieldsByteForByte(t *testing.T) {
	record := aRecord("f1")
	record.Summary = "`Decode`'s error is dropped at   `handler.go:42`."
	record.Evidence = "The call is `v, _ := dec.Decode(&body)` — *second* result discarded."

	assert.Equal(t, record.Summary+"\n\n"+record.Evidence, bodyOf(t, record))
}

// A suggestion stays beneath the prose it belongs to, so §7.1.3's block
// follows the evidence rather than splitting the two fields apart.
func TestTheSuggestionFollowsTheInitialBody(t *testing.T) {
	record := suggesting(finding.OriginAgent)

	assert.Equal(t,
		record.Summary+"\n\n"+record.Evidence+"\n\n```suggestion\n"+record.Suggestion+"\n```",
		bodyOf(t, record))
}

// An empty field contributes no paragraph. §6.1.3 makes both required, so a
// stored record carries both, but the renderer does not lean on that: a body
// opening or closing on a blank paragraph would be a line of nothing cr put in
// front of the reader.
func TestAnEmptyFieldContributesNoParagraph(t *testing.T) {
	noEvidence := aRecord("f1")
	noEvidence.Evidence = ""
	assert.Equal(t, noEvidence.Summary, bodyOf(t, noEvidence))

	noSummary := aRecord("f1")
	noSummary.Summary = ""
	assert.Equal(t, noSummary.Evidence, bodyOf(t, noSummary))
}
