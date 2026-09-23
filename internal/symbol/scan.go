package symbol

import (
	"strings"
	"unicode/utf8"
)

// signatureLines bounds how far past a declaration the parameter scan reads.
//
// A signature spanning more lines than this is either formatted very unusually
// or not a signature at all — an unbalanced paren inside a string the scanner
// misread — and the bound is what keeps the second case from walking the rest
// of the file. A count read from a truncated scan is wrong in the same
// direction as a miss: §4.3.2 requires an exact parameter-count match, so a
// wrong count drops the candidate rather than asserting anything about it.
const signatureLines = 40

// commentPrefixes begin a line no declaration is written on. The Go rules are
// anchored at column zero and need none of this; PHP's are not, so a docblock's
// ` * function handle(` would otherwise be indexed as a method that does not
// exist.
var commentPrefixes = []string{"//", "/*", "*", "#"}

// scan reads one file's declarations, in the order the file writes them.
func (l language) scan(file File) []Decl {
	decls := make([]Decl, 0, 8)
	lastClass := -1
	for at, text := range file.Lines {
		if commented(text) {
			continue
		}
		decl, matched := l.match(file, at)
		if !matched {
			continue
		}
		decls, lastClass = l.place(decls, decl, lastClass)
	}
	return decls
}

// match applies the language's rules to one line, first match winning, and
// reports the declaration it found.
func (l language) match(file File, at int) (Decl, bool) {
	text := file.Lines[at]
	for _, r := range l.rules {
		found := r.re.FindStringSubmatchIndex(text)
		if found == nil {
			continue
		}
		name := text[found[2]:found[3]]
		if r.reserved[name] || (r.rest != nil && !r.rest.MatchString(text[found[1]:])) {
			continue
		}
		decl := Decl{
			Path: file.Path,
			Line: at + 1,
			Name: name,
			Kind: r.kind,
		}
		if r.signature {
			decl.Params = countParams(file.Lines, at, found[1], l.lifetimes)
			// A receiver is written on the declaration's own line, so the
			// count already holds it and cannot fall below zero.
			if l.receiver != nil && l.receiver.MatchString(text[found[1]:]) {
				decl.Params--
			}
		}
		return decl, true
	}
	return Decl{}, false
}

// place appends a declaration and applies the two rules a single line cannot
// answer on its own: whether a function is a method, and where a class's
// declared parameter count comes from.
//
// lastClass is the index in decls of the class the file is inside, or -1 before
// the first one. It comes back updated so scan carries no state of its own.
func (l language) place(decls []Decl, decl Decl, lastClass int) (placed []Decl, inClass int) {
	if l.classScoped && decl.Kind == Function && lastClass >= 0 {
		decl.Kind = Method
	}
	if l.constructor != "" && decl.Name == l.constructor && lastClass >= 0 {
		decls[lastClass].Params = decl.Params
	}
	decls = append(decls, decl)
	if decl.Kind == Class {
		lastClass = len(decls) - 1
	}
	return decls, lastClass
}

// commented reports whether a line begins a comment, so a declaration written
// inside one is not indexed as a declaration.
func commented(text string) bool {
	trimmed := strings.TrimLeft(text, " \t")
	for _, prefix := range commentPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

// countParams reads §4.3.2's declared parameter count out of the signature that
// opens at lines[at][from:], which is the byte just past the parameter list's
// `(`.
//
// The count is the parameters that hold anything, separated by commas at the
// list's own depth — not the commas plus one. A Go signature closed on its own
// line ends `second string,` before the `)`, so counting separators would make
// every multi-line signature declare one parameter more than it has, and
// §4.3.2 matches the count exactly: an over-count drops every candidate that
// qualified. Depth and quotes are tracked for the mirror reason, because a
// default of `','` and a type of `map[string]int` each carry a comma that
// separates nothing.
func countParams(lines []string, at, from int, lifetimes bool) int {
	s := paramScanner{depth: 1, lifetimes: lifetimes}
	for row := at; row < len(lines) && row < at+signatureLines && !s.done; row++ {
		text := lines[row]
		if row == at {
			text = text[from:]
		}
		s.consume(text)
	}
	return s.params()
}

// paramScanner walks a parameter list one byte at a time, tracking how deep it
// is, whether it is inside a string, and whether the parameter it is in has
// held anything yet.
//
// It works on bytes rather than runes on purpose: every character it reacts to
// is ASCII, and no byte of a multi-byte UTF-8 rune can equal one, so a
// parameter named in any script passes through as content.
type paramScanner struct {
	// depth is the bracket depth, starting at 1 inside the parameter list
	// and reaching 0 at the paren that closes it.
	depth int
	// count is how many parameters have been closed off so far.
	count int
	// segment is whether the parameter being read has held anything but
	// whitespace.
	segment bool
	// quote is the quote character the scan is inside, or 0.
	quote byte
	// escaped is whether the previous byte was a backslash inside a quote.
	escaped bool
	// done is whether the list has closed.
	done bool
	// lifetimes is the language's, see language.lifetimes.
	lifetimes bool
}

// consume walks one line of the signature.
func (s *paramScanner) consume(text string) {
	for at := 0; at < len(text) && !s.done; at++ {
		if s.lifetime(text, at) {
			s.segment = true
			continue
		}
		s.step(text[at])
	}
}

// lifetime reports whether the byte at text[at] is the quote of a lifetime,
// which is content of the parameter rather than the start of a string.
func (s *paramScanner) lifetime(text string, at int) bool {
	return s.lifetimes && s.quote == 0 && !s.escaped && text[at] == '\'' && !charLiteral(text, at)
}

// charLiteral reports whether the single quote at text[at] opens a character
// literal: an escape, or one character and the quote that closes it. Anything
// else, in a language with lifetimes, is one.
func charLiteral(text string, at int) bool {
	rest := text[at+1:]
	if strings.HasPrefix(rest, `\`) {
		return true
	}
	_, size := utf8.DecodeRuneInString(rest)
	return size > 0 && size < len(rest) && rest[size] == '\''
}

// step advances the scan by one byte.
func (s *paramScanner) step(c byte) {
	switch {
	case s.escaped:
		s.escaped = false
	case s.quote != 0:
		s.inQuote(c)
	case c == '\'' || c == '"' || c == '`':
		s.quote, s.segment = c, true
	case c == '(' || c == '[' || c == '{':
		s.depth, s.segment = s.depth+1, true
	case c == ')' || c == ']' || c == '}':
		s.closeBracket()
	case c == ',' && s.depth == 1:
		s.separate()
	case c != ' ' && c != '\t':
		s.segment = true
	}
}

// inQuote advances the scan by one byte inside a string, where nothing but the
// closing quote and its escape means anything.
func (s *paramScanner) inQuote(c byte) {
	switch c {
	case '\\':
		s.escaped = true
	case s.quote:
		s.quote = 0
	}
}

// closeBracket leaves one bracket. Leaving the parameter list's own closes the
// parameter it was reading; leaving a nested one is content.
func (s *paramScanner) closeBracket() {
	s.depth--
	if s.depth > 0 {
		s.segment = true
		return
	}
	s.separate()
	s.done = true
}

// separate ends the parameter being read, counting it when it held anything.
func (s *paramScanner) separate() {
	if s.segment {
		s.count++
	}
	s.segment = false
}

// params is the count the scan arrived at.
//
// A parameter still open counts, which is the case where the list never closed:
// the bound above cut the scan short, or the signature is not one. The count is
// wrong there either way, and §4.3.2's exact match turns a wrong count into a
// candidate not offered rather than into anything asserted.
func (s *paramScanner) params() int {
	if s.segment {
		return s.count + 1
	}
	return s.count
}
