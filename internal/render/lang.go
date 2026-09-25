// Package render holds the language every author-facing body is written in.
//
// §8.1.1 configures that language as `render.lang`, defaulting to `en` since
// v0.14.0 and to `tr` before it. §8.1.4 then requires the question label — the
// one line §6.3's forcing reaches the reader through — to be built in per
// `render.lang` and to be
// unconfigurable. That cannot hold over a free-form string: an unknown language
// has no built-in label, and the forcing would reach the author through nothing
// at all. So v0.1 enumerates exactly two languages, a value outside them aborts
// with exit code 3 naming the setting, and the label text for each is fixed
// here rather than left to whatever renders it.
//
// The package holds the domain, the built-in label table, the set of bodies the
// language governs, and the cr-owned regions rendered from them: the question
// label, the §8.1.6 provenance region and the §8.1.7 evidence region. It also
// holds the §8.4.3 review body, whose framing is built in per language too.
package render

import (
	"fmt"
	"slices"
	"strings"
)

// Setting is the configuration key §8.1.1 reads the language from. The abort
// below names it from here, so the message cannot name a key that no longer
// exists, and §2.7's built-in default table takes its key from the same
// constant.
const Setting = "render.lang"

// Lang is one of the languages §8.1.1 lets Setting name.
//
// It is a struct around an unexported code for the reason finding.State is one:
// a defined string type is open — `var l Lang = "de"` compiles anywhere in the
// tree, because an untyped constant is assignable to it — and the enumeration
// would then be a convention rather than a fence. A struct whose only field is
// unexported can be built nowhere outside this package, and there is no
// conversion into it, so a language that is not one of the two below is
// unrepresentable. Adding one is an edit to this file, where the label table of
// label.go is waiting: a third language that joins the enumeration without
// joining the table fails
// TestEveryLanguageCarriesABuiltInQuestionLabelForEveryGrade rather than
// shipping a question the reader cannot read.
//
// The cost is that the two are `var` rather than `const`, since Go has no
// constant of struct type. Reassigning one is not widening the enumeration — it
// is sabotage that breaks every test at once — and the hole it leaves is much
// smaller than the one it closes.
type Lang struct{ code string }

// The two languages v0.1 renders.
var (
	// LangTR is Turkish.
	LangTR = Lang{"tr"}
	// LangEN is English, the default §8.1.1 gives Setting, which is also
	// the language a record is stored in per §8.1.2 — a body nobody
	// rewrites is already in it.
	LangEN = Lang{"en"}
)

// langs is the closed set, the default first.
var langs = []Lang{LangEN, LangTR}

// String returns the language's code, which is what it goes by in Setting.
func (l Lang) String() string {
	return l.code
}

// Valid reports whether l is one of the two. The zero Lang is not: a value that
// has never been through ParseLang names no language, and no label is built in
// for it.
func (l Lang) Valid() bool {
	return slices.Contains(langs, l)
}

// Langs returns the enumeration, the default first. The result is a copy, so a
// caller can neither widen the set nor reorder it.
func Langs() []Lang {
	return append(make([]Lang, 0, len(langs)), langs...)
}

// UnknownLangError reports a Setting value naming no language v0.1 renders.
//
// It carries the value so the user can see what was rejected, and names the
// setting so they know which key of §2.7 to edit.
type UnknownLangError struct {
	// Value is the offending code, exactly as the layer wrote it.
	Value string
}

func (e *UnknownLangError) Error() string {
	codes := make([]string, 0, len(langs))
	for _, known := range langs {
		codes = append(codes, known.code)
	}
	return fmt.Sprintf(
		"%s is %q, which is not a language cr renders; v0.3 has exactly %s, "+
			"whose §8.1.4 question labels are built in rather than configured — set %s to one of them",
		Setting, e.Value, strings.Join(codes, " and "), Setting,
	)
}

// ParseLang resolves a code into the language it names. It is the only way into
// the type from a string, so every Lang that exists came either from the
// enumeration above or through this check.
func ParseLang(code string) (Lang, error) {
	for _, known := range langs {
		if known.code == code {
			return known, nil
		}
	}
	return Lang{}, &UnknownLangError{Value: code}
}
