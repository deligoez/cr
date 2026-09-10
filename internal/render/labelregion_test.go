package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// §8.1.4 through §8.1.3: the label line is the first owned region, the table's
// row between the label pair and nothing else, for every language and every
// grade.
//
// The recovery half is what makes it cr-owned rather than merely cr-written:
// AgentRegion takes the whole region out, so an edit to the line never reaches
// the author and the question-mark check of §8.1.5 never reads it.
func TestTheLabelRegionIsTheBuiltInLineBetweenItsPair(t *testing.T) {
	for _, lang := range Langs() {
		for _, grade := range grades {
			text, ok := QuestionLabel(lang, grade)
			require.True(t, ok)

			region, err := QuestionLabelRegion(lang, grade)
			require.NoError(t, err)
			assert.Equal(t, "<!-- cr:label -->\n"+text+"\n<!-- cr:/label -->", region,
				"%s, %s: the built-in row, delimited by §8.1.3's label pair", lang, grade)

			comment := Comment{Label: region, Body: "Is the error dropped?"}
			assert.Equal(t, "Is the error dropped?", AgentRegion(comment.String()),
				"the label is owned, so recovery leaves the agent body alone")
		}
	}
}

// A pair the built-in table has no row for is refused, never rendered as a
// question without its label — which would post exactly the comment §6.3's
// forcing exists to prevent.
func TestAPairTheTableHasNoRowForIsRefused(t *testing.T) {
	for name, pair := range map[string]struct {
		lang  Lang
		grade finding.Grade
	}{
		"a language nothing parsed":  {Lang{}, finding.GradeArgued},
		"a grade §6.2 does not have": {LangEN, finding.Grade("verified")},
		"no grade at all":            {LangTR, finding.Grade("")},
	} {
		t.Run(name, func(t *testing.T) {
			region, err := QuestionLabelRegion(pair.lang, pair.grade)

			var missing *NoLabelError
			require.ErrorAs(t, err, &missing)
			assert.Equal(t, pair.lang, missing.Lang)
			assert.Equal(t, pair.grade, missing.Grade)
			assert.Empty(t, region)
		})
	}
}
