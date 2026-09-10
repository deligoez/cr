package draft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// §6.3's forcing reaches the reader only if the body is rewritten into a
// question, and §8.1.5 is what holds the agent to it.
//
// The record is the one §6.3 forces: `argued`, drafted as a question, its
// initial body the English summary and evidence. Read back as it was drafted,
// its agent region is a statement under a question label, and §8.1.5 refuses
// it naming the record. Rewritten in `draft.md` into a question, it passes —
// with the label regenerated rather than read back, so nothing about the
// rewrite could have touched the register line.
func TestAForcedQuestionIsRefusedUntilItsBodyAsks(t *testing.T) {
	forced := aRecord("f3")
	forced.Kind, forced.Grade = finding.KindQuestion, finding.GradeArgued

	rendered, err := Render([]*finding.Finding{forced}, render.LangEN, nil)
	require.NoError(t, err)
	_, comment, found := strings.Cut(rendered, " -->\n\n")
	require.True(t, found)

	err = render.ValidatePostBody(forced.ID, forced.Kind, render.AgentRegion(comment))
	var refused *render.BodyError
	require.ErrorAs(t, err, &refused, "§8.1.5: the unrewritten initial body is a statement")
	assert.Equal(t, "f3", refused.Record)

	rewritten := strings.Replace(comment, forced.Summary, "Is the error Decode returns dropped?", 1)
	assert.NoError(t,
		render.ValidatePostBody(forced.ID, forced.Kind, render.AgentRegion(rewritten)),
		"the agent's rewrite into a question is what the post accepts")
}
