package draft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// headerFacts is a round of four units under three active roles, one of them
// with a gap and one oversized, counted against a cap of twenty.
var headerFacts = HeaderFacts{
	MaxComments: 20,
	Coverage:    coverage.Rows{Units: 4, Complete: 3, Gaps: 1, Oversized: 1, Roles: 3},
}

// fileOf is File over records in English under headerFacts.
func fileOf(t *testing.T, facts HeaderFacts, records ...*finding.Finding) string {
	t.Helper()
	rendered, err := File(records, render.LangEN, nil, facts)
	require.NoError(t, err)
	return rendered
}

// headerOfFile is the header region of a rendered file: everything up to the
// first comment close, which must be the header's own.
func headerOfFile(t *testing.T, rendered string) string {
	t.Helper()
	end := strings.Index(rendered, "-->")
	require.GreaterOrEqual(t, end, 0, "the header closes")
	return rendered[:end+len("-->")]
}

// §7.1.4: the file opens with a summary header listing counts, the coverage
// state, and the comment count against post.max_comments, as a comment that is
// not posted.
//
// "Not posted" is asserted as a shape: the header is one HTML comment, opened
// on the file's first line and closed before the first block's marker, so no
// line of it is prose a renderer shows or a block reader could take as a body.
func TestTheFileOpensWithTheSummaryHeader(t *testing.T) {
	question := aRecord("f2")
	question.Kind, question.Grade, question.Severity = finding.KindQuestion, finding.GradeArgued, finding.SeverityMedium
	question.Summary = "Does the retry back off?"

	rendered := fileOf(t, headerFacts, aRecord("f1"), question, aRecord("f3"))

	require.True(t, strings.HasPrefix(rendered, "<!-- cr:summary\n"),
		"§7.1.4: the file opens with the header")
	header := headerOfFile(t, rendered)
	assert.Less(t, len(header), strings.Index(rendered, "<!-- cr:record "),
		"the header closes before the first block opens")
	assert.True(t, strings.HasSuffix(header, "\n-->"), "the close is a line of its own")
	assert.Equal(t, 1, strings.Count(header, "<!--"), "the header is exactly one comment")

	for _, line := range []string{
		"records: 3 queued",
		"kind: finding 2, question 1",
		"severity: critical 0, high 2, medium 1, low 0",
		"grade: probed 0, cited 2, argued 1",
		"coverage: 4 unit(s) against 3 active role(s): 3 with a complete row of cells, 1 with gaps, 1 oversized",
		"comments: 3 comments queued against post.max_comments 20",
	} {
		assert.Contains(t, strings.Split(header, "\n"), line)
	}
	assert.Equal(t, renderOf(t, aRecord("f1"), question, aRecord("f3")),
		strings.TrimPrefix(rendered, header+"\n\n"),
		"beneath the header and a blank line, the blocks are exactly what Render gives")
}

// The header's comment count is §1.6.2's, from the call the cap check makes,
// at the cap and one past it. Past it, the header names the excess in the words
// the block itself will use, so the reviewer triages against the number that
// will stop `cr post`.
func TestTheHeaderCountsCommentsAsTheCapCheckDoes(t *testing.T) {
	for _, count := range []int{20, 21} {
		queued := make([]*finding.Finding, 0, count)
		for range count {
			queued = append(queued, aRecord("f1"))
		}
		capped := finding.CommentCapFor(queued, headerFacts.MaxComments)

		header := headerOfFile(t, fileOf(t, headerFacts, queued...))

		assert.Contains(t, header, "\ncomments: "+capped.Disclosure()+"\n", "at %d", count)
	}
	over := fileOf(t, HeaderFacts{MaxComments: 2}, aRecord("f1"), aRecord("f2"), aRecord("f3"))
	assert.Contains(t, headerOfFile(t, over),
		"3 comments queued against post.max_comments 2, 1 over the cap")
}
