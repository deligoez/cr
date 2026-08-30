package render

import (
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/finding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// grades is §6.2's three, written out because internal/finding holds the
// vocabulary as constants rather than as a set. §6.2 fixes it at exactly these,
// and a question can carry any of them: §6.3 forces `argued` to a question, and
// nothing stops a `probed` or `cited` record from being written as one.
var grades = []finding.Grade{finding.GradeProbed, finding.GradeCited, finding.GradeArgued}

// §8.1.4 requires the question label to be built in per render.lang. Built in
// means the table is total over the enumeration: every language cr accepts has
// a line for every grade a question can carry, so §6.3's forcing reaches the
// reader whichever of the two the repository is configured for.
//
// The two exact lines are asserted rather than only their shape. The text is
// fixed in the implementation precisely so that changing it is a diff someone
// reads, and a test that checked only "non-empty and names the grade" would let
// the register — the word that makes it a question — be edited away in silence.
func TestEveryLanguageCarriesABuiltInQuestionLabelForEveryGrade(t *testing.T) {
	require.Len(t, questionLabels, len(Langs())*len(grades),
		"the table is exactly the cross product, with no language and no grade missing or twice")

	seen := make(map[string]bool, len(questionLabels))
	for _, lang := range Langs() {
		for _, grade := range grades {
			text, ok := QuestionLabel(lang, grade)
			require.Truef(t, ok, "%s has no built-in label for a %s question", lang, grade)
			assert.NotEmpty(t, text)
			assert.Containsf(t, text, string(grade),
				"§8.1.4: the line names the grade, and %s keeps the stored English word", lang)
			assert.NotContains(t, text, "\n", "§8.1.4 prepends a label line, not a block")
			assert.Falsef(t, seen[text], "%s and %s share a line", lang, grade)
			seen[text] = true
		}
	}

	tr, _ := QuestionLabel(LangTR, finding.GradeArgued)
	assert.Equal(t, "**Soru** — kanıt düzeyi: gerekçeye dayalı (argued)", tr)
	en, _ := QuestionLabel(LangEN, finding.GradeArgued)
	assert.Equal(t, "**Question** — evidence grade: argued", en)
	assert.True(t, strings.HasPrefix(tr, "**Soru**") && strings.HasPrefix(en, "**Question**"),
		"§8.1.4: the line names the register, in the language the author reads")

	_, ok := QuestionLabel(Lang{}, finding.GradeArgued)
	assert.False(t, ok,
		"a language nothing parsed has no built-in label, and the lookup says so rather than defaulting")
}
