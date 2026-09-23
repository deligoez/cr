// Package symbol builds §4.3.1's head symbol index: where every function,
// method, and class of one head is declared, and how many parameters it
// declares.
//
// The index is mechanical and nothing here judges. §4.3.1 attaches candidates
// and §4.3.2 ranks them; whether two symbols mean the same thing is the agent's
// call, which invariant 1 and P5 both put outside cr. So a scanner here answers
// one question — what does this file declare, and where — and stops.
//
// # Why a line scanner rather than a parser per language
//
// `symbols.lang` is a free string a profile writes, so the set of languages is
// open. A real parser per language would be a dependency per language and a
// tree cr has no other use for, while the index needs four facts about each
// declaration: name, path, line, and declared parameter count. Every one of
// them is on or just after the declaration's own line in the languages v0.1
// ships a profile for.
//
// What that costs is stated rather than hidden. A scanner sees a declaration
// spelled in a way its rules do not cover, or a declaration inside a raw string,
// as a miss and a false hit respectively — and §4.3.4 is what makes the price
// bearable in one direction: a reinvention item is a question, so a spurious
// candidate costs the reader a question they answer with "no", while a missed
// one costs a question never asked. Neither is a wrong assertion, which is the
// expense the trust economy is written against.
package symbol

import (
	"cmp"
	"slices"
)

// Kind is what a declaration declares, in §4.3.1's three words.
type Kind string

const (
	// Function is a free function.
	Function Kind = "function"
	// Method is a function declared inside a class.
	Method Kind = "method"
	// Class is a class, and every construct a language spells the same
	// way — an interface, a trait, an enum, a named struct type.
	Class Kind = "class"
)

// Decl is one declaration the head holds.
//
// Line is where the declaration is written, which is the line §4.3.3 cites as
// `path:line` and the line §4.3.1 tests against the diff's changed lines. It is
// not a span: the index answers where a symbol is declared here, and keeps the
// lines each body covers apart, for unit.SymbolIndex's different question of
// what encloses a line (span.go).
type Decl struct {
	// Path is the file the declaration sits in, repository-relative and
	// slash-separated, as git spells a path.
	Path string `json:"path"`
	// Line is the one-based line the declaration is written on.
	Line int `json:"line"`
	// Name is the declared name, spelled as the source spells it. §4.3.2's
	// comparison name is derived from it and never stored in its place,
	// because §4.3.3 cites the symbol a reader will search for.
	Name string `json:"name"`
	// Kind is what was declared.
	Kind Kind `json:"kind"`
	// Params is the declared parameter count of §4.3.2: the parameters in
	// a function's or method's signature, and for a class the parameter
	// count of its constructor, or zero when it declares none.
	Params int `json:"params"`
}

// File is one head file handed to the index: its path, and the lines the head
// holds at that path.
type File struct {
	// Path is the file's repository-relative path.
	Path string
	// Lines are the file's lines, numbered from one.
	Lines []string
}

// Index is §4.3.1's head symbol index.
//
// Decls is ordered by path ascending then line ascending, which is the tail of
// §4.3.2's ordering. Ordering the index once means a ranking that ties on
// similarity is already in spec order without sorting the pool again, and it
// makes two runs over one head produce the same output per §2.1.1 — git's
// listing order is git's, not cr's.
type Index struct {
	// Lang is the `symbols.lang` the index was built for.
	Lang string `json:"lang"`
	// Decls are every declaration the head holds, in path then line order.
	// It is empty rather than nil when the head declares nothing, per §12.
	Decls []Decl `json:"decls"`
	// files are the paths the index was built over, including a file that
	// declares nothing, which is §3.4.3's per-file question.
	files map[string]bool
	// spans are, per path, the lines each declaration covers where the scan
	// could bound its body, which is §3.4.4's symbol branch's question.
	spans map[string][]span
}

// Build indexes files in the language lang names.
//
// It reports false when cr has no scanner for lang, which is the state §4.3.1
// turns into a §4.5.4 unavailability rather than into an error or into an empty
// index. The two are not the same answer and must not share a shape: an empty
// index says the head declares nothing, and reporting "cr looked and found no
// symbols" for a language cr cannot read is the silent skip §4.3.1 forbids.
func Build(lang string, files []File) (*Index, bool) {
	rules, known := languages[lang]
	if !known {
		return nil, false
	}
	index := &Index{
		Lang: lang, Decls: make([]Decl, 0, len(files)),
		files: make(map[string]bool, len(files)), spans: make(map[string][]span, len(files)),
	}
	for _, file := range files {
		found := rules.scan(file)
		index.Decls = append(index.Decls, found...)
		index.cover(file, found, rules.lifetimes)
	}
	slices.SortStableFunc(index.Decls, func(a, b Decl) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line))
	})
	return index, true
}

// Supported reports whether cr has a scanner for lang, so a caller can name the
// reason §4.5.4 owes the reader before spending a single git read on a head it
// cannot index.
func Supported(lang string) bool {
	_, known := languages[lang]
	return known
}
