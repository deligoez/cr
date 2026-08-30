package render

import "github.com/deligoez/cr/internal/finding"

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
func QuestionLabel(lang Lang, grade finding.Grade) (string, bool) {
	for _, label := range questionLabels {
		if label.lang == lang && label.grade == grade {
			return label.text, true
		}
	}
	return "", false
}
