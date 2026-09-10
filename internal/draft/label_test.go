package draft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// §8.1.4: cr prepends the fixed label line to every `kind=question` comment,
// naming the register and the grade, in `render.lang` — and to no finding.
//
// Every language and every grade is drafted, because "every question" is the
// claim and a question can carry any of §6.2's three: §6.3 forces `argued`
// into the register, and nothing stops a `probed` or `cited` record from being
// written as a question. The whole comment is compared, so the label is shown
// to be first and the agent body to follow it untouched.
func TestEveryQuestionOpensWithItsLabelAndNoFindingCarriesOne(t *testing.T) {
	grades := []finding.Grade{finding.GradeProbed, finding.GradeCited, finding.GradeArgued}
	for _, lang := range render.Langs() {
		for _, grade := range grades {
			question := aRecord("f1")
			question.Kind, question.Grade = finding.KindQuestion, grade
			asserting := aRecord("f2")
			asserting.Grade = grade

			rendered, err := Render([]*finding.Finding{question, asserting}, lang)
			require.NoError(t, err)

			label, err := render.QuestionLabelRegion(lang, grade)
			require.NoError(t, err)
			initial := question.Summary + "\n\n" + question.Evidence
			assert.Contains(t, rendered, markerOf(question).String()+"\n\n"+label+"\n\n"+initial+"\n",
				"%s, %s: the label opens the question's comment, and the body follows it", lang, grade)

			_, findingBlock, found := strings.Cut(rendered, markerOf(asserting).String())
			require.True(t, found)
			assert.NotContains(t, findingBlock, "<!-- cr:label",
				"%s, %s: a finding carries no question label", lang, grade)
		}
	}
}

// The label a drafted question carries is owned, so the agent region the
// question's body is recovered as is the initial body alone. §8.1.5's check and
// §7.1.5's `rendered.json` read that region, and the label's own words never
// count toward either.
func TestTheDraftedLabelIsNotPartOfTheAgentRegion(t *testing.T) {
	question := aRecord("f1")
	question.Kind, question.Grade = finding.KindQuestion, finding.GradeArgued

	rendered, err := Render([]*finding.Finding{question}, render.LangTR)
	require.NoError(t, err)
	_, comment, found := strings.Cut(rendered, " -->\n\n")
	require.True(t, found)

	assert.Equal(t, question.Summary+"\n\n"+question.Evidence, render.AgentRegion(comment))
}

// A question the table cannot label stops the draft naming the record, rather
// than reaching the reviewer as a question without its register.
func TestAQuestionTheTableCannotLabelStopsTheDraft(t *testing.T) {
	question := aRecord("f4")
	question.Kind, question.Grade = finding.KindQuestion, finding.GradeArgued

	rendered, err := Render([]*finding.Finding{question}, render.Lang{})

	var missing *render.NoLabelError
	require.ErrorAs(t, err, &missing)
	assert.Contains(t, err.Error(), "f4", "the refusal names the record")
	assert.Empty(t, rendered)
}
