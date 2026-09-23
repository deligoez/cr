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
	// rest, when set, must match the line from the end of re's match, and
	// a line it does not match is not this rule's declaration. It is what
	// tells `const f = (a) => a` from `const f = (a + b)`, and a method
	// from a call, where the part of the line up to the paren reads alike.
	rest *regexp.Regexp
	// reserved names are words the pattern's position admits and no
	// declaration can carry, such as the `if` of `if (ready) {`.
	reserved map[string]bool
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
	// constructor is the method name a language declares a class's
	// parameters in, so §4.3.2's "for a class it is the parameter count of
	// its constructor" is filled in as the scan passes it.
	constructor string
	// receiver matches the start of a parameter list whose first parameter
	// is the receiver, which is then not counted. Go writes its receiver
	// outside the list and PHP never writes one, so without this a Rust
	// method would declare one parameter more than the same method in
	// either, and §4.3.2's exact match would drop it.
	receiver *regexp.Regexp
	// syntax is what the byte scanners need to know about the language's
	// strings and comments.
	syntax
}

// syntax is the lexical difference between the languages the byte scanners of
// scan.go and span.go walk. Each flag is one way a scanner that did not know
// it would read a string or a comment as code, or code as one — and a brace or
// a comma read wrongly moves a body's end past the next declaration or
// miscounts a parameter list.
type syntax struct {
	// hashComments is true where `#` opens a line comment (PHP). In
	// TypeScript it opens a private name and in Rust an attribute or a raw
	// string, so reading it as a comment there skips the braces after it.
	hashComments bool
	// lineQuotes is true where a single- or double-quoted string cannot span
	// a line (TypeScript and JavaScript), so a quote left open at a line's
	// end — an apostrophe in JSX text — closes there instead of swallowing
	// the code below it.
	lineQuotes bool
	// rawStrings is true for Rust's `r"…"` and `r#"…"#`, whose contents are
	// read verbatim until the quote and the same number of `#`.
	rawStrings bool
	// lifetimes is true for a language where a single quote not closed a
	// character later is a lifetime, `'a`, rather than the start of a
	// string: read as a string, Rust's `&'a str` would swallow every comma
	// and brace after it.
	lifetimes bool
	// angles is true where a parameter's type nests in `<>`
	// (`Map<string, number>`), so a comma inside one separates nothing.
	angles bool
}

// languages is every `symbols.lang` value cr can build an index for. A profile
// naming anything else gets §4.3.1's unavailability, which is the honest answer
// and the reason Supported exists.
var languages = map[string]language{
	"go":         goLanguage(),
	"php":        phpLanguage(),
	"typescript": typescriptLanguage(),
	"rust":       rustLanguage(),
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
		syntax:      syntax{hashComments: true},
		rules: []rule{
			{kind: Class, re: regexp.MustCompile(
				`^\s*(?:(?:abstract|final|readonly)\s+)*(?:class|interface|trait|enum)\s+(` + ident + `)`)},
			{kind: Function, signature: true, re: regexp.MustCompile(
				`^\s*(?:(?:public|protected|private|static|final|abstract|readonly)\s+)*` +
					`function\s*&?\s*(` + ident + `)\s*\(`)},
		},
	}
}

// typescriptLanguage scans TypeScript and JavaScript, and a Vue single-file
// component's script block, which is the same language between two tags: the
// template's lines match none of the rules.
//
// A method has no keyword of its own, so it is recognised by shape — an
// indented name, a parameter list that holds no nested paren, and a body
// opened on the same line — and the shape is shared with a call handed a
// callback and with `if (ready) {`. rest turns the first away, since a
// callback's list holds a paren, and reserved the second. What the shape
// misses is a method whose parameters span lines or carry a call as a default,
// and a private `#name(`: a symbol absent from the index, never one invented.
//
// An arrow function is a function when a `const` or `let` binds it and its
// parameter list is followed by its `=>`, a return type allowed between. A
// parenthesised expression that merely reaches an arrow later on the line,
// `const ids = (items ?? []).map((i) => i.id)`, is not one. One whose
// parameters span lines is missed.
//
// A generic list is matched lazily up to a `>` no `=` or `-` stands before,
// so a constraint holding an arrow type or a nested generic is read whole.
func typescriptLanguage() language {
	methodOpens := regexp.MustCompile(`^[^()]*\)[^;=(]*\{\s*\}?\s*$`)
	arrowFollows := regexp.MustCompile(`^(?:[^()]|\([^()]*\))*\)\s*(?::[^=]*)?=>`)
	const generic = `(?:<.*?[^=-]>\s*)?`
	return language{
		constructor: "constructor",
		syntax:      syntax{lineQuotes: true, angles: true},
		rules: []rule{
			{kind: Class, re: regexp.MustCompile(
				`^\s*(?:export\s+)?(?:default\s+)?(?:declare\s+)?(?:abstract\s+)?` +
					`(?:class|interface|enum)\s+(` + ident + `)`)},
			{kind: Function, signature: true, re: regexp.MustCompile(
				`^\s*(?:export\s+)?(?:default\s+)?(?:declare\s+)?(?:async\s+)?` +
					`function\s*\*?\s*(` + ident + `)\s*` + generic + `\(`)},
			{kind: Function, signature: true, rest: arrowFollows, re: regexp.MustCompile(
				`^\s*(?:export\s+)?(?:const|let)\s+(` + ident + `)\s*(?::[^=]*)?=\s*(?:async\s+)?` +
					generic + `\(`)},
			{kind: Method, signature: true, rest: methodOpens, reserved: jsReserved, re: regexp.MustCompile(
				`^\s+(?:(?:public|private|protected|static|async|readonly|override|abstract|get|set)\s+)*` +
					`(` + ident + `)\s*` + generic + `\(`)},
		},
	}
}

// jsReserved are the words that open a line the way a method does, `name(...)
// {`, and are control flow rather than a declaration.
var jsReserved = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true,
	"with": true, "function": true, "return": true, "await": true, "new": true,
	"typeof": true, "do": true, "else": true,
}

// rustLanguage scans Rust.
//
// A `fn` at column zero is a function and an indented one is a method. That
// reads a function nested in another, or declared inside `mod tests`, as a
// method, which mislabels a Kind and loses nothing, as PHP's classScoped
// heuristic does. A `struct`, `enum`, `trait` or `union` is indexed as a Class
// with no parameters, for the reason a Go type is: Rust has no constructors.
func rustLanguage() language {
	const qualifiers = `(?:pub(?:\([^)]*\))?\s+)?(?:(?:const|async|unsafe|extern\s+"[^"]*")\s+)*`
	// The generic list is matched lazily up to a `>` the parameter list
	// follows and no `-` stands before, so `<T: Into<String>>` and
	// `<F: Fn() -> ()>` are each read whole.
	const signature = `fn\s+(` + ident + `)\s*(?:<.*?[^-]>\s*)?\(`
	return language{
		syntax:   syntax{rawStrings: true, lifetimes: true, angles: true},
		receiver: regexp.MustCompile(`^\s*(?:&\s*(?:'` + ident + `\s+)?)?(?:mut\s+)?self\b`),
		rules: []rule{
			{kind: Class, re: regexp.MustCompile(
				`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:struct|enum|trait|union)\s+(` + ident + `)`)},
			{kind: Function, signature: true, re: regexp.MustCompile(`^` + qualifiers + signature)},
			{kind: Method, signature: true, re: regexp.MustCompile(`^\s+` + qualifiers + signature)},
		},
	}
}
