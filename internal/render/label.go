package render

import (
	"fmt"

	"github.com/deligoez/cr/internal/finding"
)

// questionLabel is one row of the built-in table: the fixed line §8.1.4 has cr
// prepend to a `kind=question` comment, for one language and one grade.
//
// The grade is part of the key because §8.1.4 has the line name the register
// **and** the grade. Both halves are what the line is for: the register is how
// §6.3's forcing reaches the reader at all, and the grade is what tells them
// how much the question rests on.
type questionLabel struct {
	lang  Lang
	grade finding.Grade
	text  string
}

// questionLabels is that table in full: every language of langs against every
// grade of §6.2, written out rather than assembled.
//
// Written out is the point. §8.1.4 makes the text built in and not
// configurable, and §2.7 protects it exactly as it protects the confirmation
// gate — so the text is a literal here, where changing it is a diff a reviewer
// reads, rather than a composition a later caller can drive to a different
// result with a different argument. There is no format string, no lookup of a
// grade's name, and nothing a profile, role, or rule can reach.
//
// Each Turkish row carries the stored English grade word in parentheses. The
// grade is what §6.1 keeps in `findings.ndjson`, and an author who reads the
// label can then find the record the label came from; translating it away would
// leave the reader a word that appears nowhere in the state cr wrote.
var questionLabels = []questionLabel{
	{LangTR, finding.GradeProbed, "**Soru** — kanıt düzeyi: deneyle sınanmış (probed)"},
	{LangTR, finding.GradeCited, "**Soru** — kanıt düzeyi: kaynak gösterilmiş (cited)"},
	{LangTR, finding.GradeArgued, "**Soru** — kanıt düzeyi: gerekçeye dayalı (argued)"},
	{LangEN, finding.GradeProbed, "**Question** — evidence grade: probed"},
	{LangEN, finding.GradeCited, "**Question** — evidence grade: cited"},
	{LangEN, finding.GradeArgued, "**Question** — evidence grade: argued"},
}

// QuestionLabel returns the built-in line §8.1.4 prepends to a question graded
// grade, in lang. The second result reports whether the table has a row for the
// pair.
//
// It is a lookup and never a composition, for the reason the table gives. The
// boolean is not ceremony: the zero Lang is representable, and a caller that
// reached one has no built-in label to prepend — which is precisely the case
// the enumeration of lang.go exists to keep out of the renderer, so it is
// reported rather than papered over with a default.
//
// The table is the comment body's, so BodyComment is asked first whether
// Setting's language governs it: a label is text of §8.1's body, and a body the
// set does not name has none in any language.
func QuestionLabel(lang Lang, grade finding.Grade) (string, bool) {
	if !BodyComment.AuthorFacing() {
		return "", false
	}
	for _, label := range questionLabels {
		if label.lang == lang && label.grade == grade {
			return label.text, true
		}
	}
	return "", false
}

// QuestionLabelRegion is §8.1.4's line as §8.1.3's first owned region: the
// built-in label for a question graded grade, in lang, between its own named
// pair.
//
// It is the region every `kind=question` comment opens with, and it is
// regenerated rather than read back, so what the reader sees is the table's
// row and never an edit of it. A pair the table has no row for is refused
// rather than rendered without a label: §6.3's forcing reaches the reader
// through this line alone, and a question without it would post as if the
// forcing had never happened.
func QuestionLabelRegion(lang Lang, grade finding.Grade) (string, error) {
	text, ok := QuestionLabel(lang, grade)
	if !ok {
		return "", &NoLabelError{Lang: lang, Grade: grade}
	}
	return labelRegion.wrap(text), nil
}

// NoLabelError reports a question the built-in table holds no §8.1.4 label
// for: a language nothing parsed, or a grade outside §6.2's three.
//
// Neither can come from a value cr wrote. Setting is checked against the
// enumeration when configuration resolves, and §6.2 computes every stored
// grade, so reaching this means the state the question was read from is not
// state cr produced — which the cli layer codes 3 with the other files cr
// cannot use.
type NoLabelError struct {
	// Lang is the language the label was asked in.
	Lang Lang
	// Grade is the grade it was asked for.
	Grade finding.Grade
}

func (e *NoLabelError) Error() string {
	return fmt.Sprintf(
		"§8.1.4 builds in no question label for language %q and grade %q, "+
			"and a question cannot reach the author without one",
		e.Lang.String(), e.Grade)
}
