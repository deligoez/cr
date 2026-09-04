package symbol

import "regexp"

// ident is an identifier as the languages below spell one. It is Unicode rather
// than ASCII because both permit a non-ASCII name, and a `\w` here would index
// `deger` and miss `değer` — a symbol silently absent from the index is a
// candidate §4.3.1 never offers and a reader never learns was skipped.
const ident = `[\p{L}_][\p{L}\p{N}_]*`

// rule matches one declaration on one line.
//
// The pattern captures the declared name in group 1. A rule whose declaration
// carries a signature ends its pattern at the `(` that opens the parameter
// list, so the match's own end is where §4.3.2's parameter count is read from,
// and the two cannot drift apart the way a second search for `(` could.
type rule struct {
	kind Kind
	// signature is true when the pattern ends at the parameter list's
	// opening paren, and false for a declaration that carries none.
	signature bool
	re        *regexp.Regexp
}

// language is the rule set one `symbols.lang` value scans with, together with
// the two facts a rule alone cannot express.
type language struct {
	// rules are tried in order, first match wins, so a more specific rule
	// is written before the rule it would otherwise be swallowed by.
	rules []rule
	// classScoped is true for a language that declares its methods with
	// the same keyword as its free functions, so a function is a method
	// exactly when a class has been declared above it in the same file.
	//
	// This is a heuristic and it is worth naming as one. It reads a file
	// with two classes and a trailing free function as declaring three
	// methods. PSR-4 — which the shipped laravel-pest profile's tree
	// follows — puts one class in a file and nothing after it, so the
	// heuristic is right for the code that profile matches, and where it
	// is wrong it mislabels a Kind rather than inventing or losing a
	// symbol. §4.3.2 ranks on the comparison name and the parameter count,
	// neither of which this touches.
	classScoped bool
	// constructor is the method name a classScoped language declares a
	// class's parameters in, so §4.3.2's "for a class it is the parameter
	// count of its constructor" is filled in as the scan passes it.
	constructor string
}

// languages is every `symbols.lang` value cr can build an index for. A profile
// naming anything else gets §4.3.1's unavailability, which is the honest answer
// and the reason Supported exists.
var languages = map[string]language{
	"go":  goLanguage(),
	"php": phpLanguage(),
}

// goLanguage scans Go.
//
// Every pattern is anchored at the start of the line, because Go declares its
// functions, methods, and named types at column zero and gofmt keeps them
// there. That anchor is doing the work a comment and string skip would
// otherwise do: `// func Greet(` and a raw string's indented `func` both fail
// it, and this project's own gate runs gofmt.
//
// A named struct or interface type is indexed as a Class. Go has no classes and
// no constructors, so §4.3.2's class rule reads directly: the type declares no
// constructor, so its parameter count is zero. Leaving types out instead would
// drop the one Go construct a second author actually re-declares by accident.
func goLanguage() language {
	return language{
		rules: []rule{
			// The receiver first: `func (r Repo) Find(` also matches
			// the plain-function pattern, at the receiver rather
			// than at the name.
			{kind: Method, signature: true, re: regexp.MustCompile(
				`^func\s+\([^()]*\)\s*(` + ident + `)\s*(?:\[[^\]]*\]\s*)?\(`)},
			{kind: Function, signature: true, re: regexp.MustCompile(
				`^func\s+(` + ident + `)\s*(?:\[[^\]]*\]\s*)?\(`)},
			{kind: Class, re: regexp.MustCompile(
				`^type\s+(` + ident + `)\s*(?:\[[^\]]*\])?\s+(?:struct|interface)\b`)},
		},
	}
}

// phpLanguage scans PHP, the language the shipped laravel-pest profile declares.
//
// The class pattern covers `interface`, `trait`, and `enum` alongside `class`:
// §4.3.1 names three kinds of declaration and PHP spells four constructs the
// one way, so folding them into Class keeps the index complete without
// inventing a fourth Kind the spec never asked for.
func phpLanguage() language {
	return language{
		classScoped: true,
		constructor: "__construct",
		rules: []rule{
			{kind: Class, re: regexp.MustCompile(
				`^\s*(?:(?:abstract|final|readonly)\s+)*(?:class|interface|trait|enum)\s+(` + ident + `)`)},
			{kind: Function, signature: true, re: regexp.MustCompile(
				`^\s*(?:(?:public|protected|private|static|final|abstract|readonly)\s+)*` +
					`function\s*&?\s*(` + ident + `)\s*\(`)},
		},
	}
}
