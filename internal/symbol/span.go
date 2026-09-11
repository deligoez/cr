package symbol

import (
	"strconv"
	"strings"
)

// span is the lines one declaration covers at the head: from the line it is
// declared on through the line its body closes on, both one-based.
type span struct {
	start, end int
	// key names the declaration uniquely within its file. The name alone
	// is not enough: two Go methods on two receivers may share one, and
	// §3.4.4 groups hunks by the symbol, not by the spelling.
	key string
}

// cover records one scanned file: that the index was built over it, and the
// span of every declaration whose body the scan could bound.
//
// A declaration whose body cannot be bounded gets no span rather than a guess.
// §3.4.3 lets clustering fall through to adjacency without an error, and that
// fallthrough costs the reader a differently shaped unit; a span that ran past
// the body would instead name a symbol for hunks it does not contain, which is
// the one thing a unit formed `by symbol` asserts.
func (x *Index) cover(file File, decls []Decl) {
	x.files[file.Path] = true
	for _, decl := range decls {
		if end, bounded := bodyEnd(file.Lines, decl.Line-1); bounded {
			x.spans[file.Path] = append(x.spans[file.Path], span{
				start: decl.Line, end: end, key: decl.Name + "@" + strconv.Itoa(decl.Line),
			})
		}
	}
}

// Indexed reports whether the index was built over path, which is the half of
// unit.SymbolIndex §3.4.3 asks. A nil index, the state of a head cr could not
// index, covers nothing.
func (x *Index) Indexed(path string) bool {
	return x != nil && x.files[path]
}

// Enclosing names the innermost declaration whose span holds the head-side line
// at path, which is the half of unit.SymbolIndex §3.4.4's symbol branch asks,
// and reports false when none does.
//
// Innermost, because a PHP class's span holds its methods' spans: a line in a
// method is enclosed by both, and naming the class would gather every edit to
// the file into one unit. Spans nest rather than overlap, so the innermost is
// the one declared last.
func (x *Index) Enclosing(path string, line int) (string, bool) {
	if x == nil {
		return "", false
	}
	found := -1
	spans := x.spans[path]
	for i := range spans {
		if spans[i].start <= line && line <= spans[i].end && (found < 0 || spans[i].start > spans[found].start) {
			found = i
		}
	}
	if found < 0 {
		return "", false
	}
	return spans[found].key, true
}

// bodyEnd finds the one-based line the body of the declaration on lines[at]
// closes on, and reports false when the scan cannot bound it.
//
// The body is the first brace opened outside any parenthesis, which skips a
// Go parameter typed `struct{...}` and reaches a PHP body opened on the line
// after its signature. A `;` before any body is a declaration without one — an
// interface or abstract method — and covers its own lines. The search for the
// opening brace gives up at a blank line or past signatureLines, since no
// signature holds either, and a brace found beyond them would be the next
// declaration's.
func bodyEnd(lines []string, at int) (int, bool) {
	s := braceScanner{}
	for row := at; row < len(lines); row++ {
		if !s.opened && (row >= at+signatureLines || strings.TrimSpace(lines[row]) == "") {
			return 0, false
		}
		s.consume(lines[row])
		if s.closed || s.bodiless {
			return row + 1, true
		}
	}
	return 0, false
}

// braceScanner walks a declaration's source for the brace that closes its
// body, skipping strings and comments so a brace inside either counts for
// nothing.
//
// What it does not see is stated rather than hidden: a PHP heredoc is read as
// code, so an unbalanced brace inside one moves the end. Where that leaves the
// body unclosed the declaration gets no span and falls through to adjacency.
type braceScanner struct {
	parens, braces int
	// opened is whether the body's brace has been seen, closed whether the
	// brace that balances it has, and bodiless whether a `;` ended the
	// declaration first.
	opened, closed, bodiless bool
	// quote is the quote character the scan is inside, or 0, and escaped
	// whether the previous byte was a backslash inside it.
	quote   byte
	escaped bool
	// block is whether the scan is inside a block comment.
	block bool
}

// consume walks one line, stopping at the byte that closes the body.
func (s *braceScanner) consume(text string) {
	for i := 0; i < len(text) && !s.closed && !s.bodiless; i++ {
		c, next := text[i], byte(0)
		if i+1 < len(text) {
			next = text[i+1]
		}
		switch {
		case s.block:
			if c == '*' && next == '/' {
				s.block, i = false, i+1
			}
		case s.quote != 0:
			s.inQuote(c)
		case c == '#' && next != '[' || c == '/' && next == '/':
			// A line comment, and not PHP 8's `#[` attribute, which
			// can sit inside a signature.
			return
		case c == '/' && next == '*':
			s.block, i = true, i+1
		case c == '\'' || c == '"' || c == '`':
			s.quote = c
		default:
			s.bracket(c)
		}
	}
}

// inQuote advances the scan by one byte inside a string. A Go raw string has
// no escapes, so a backslash ends nothing there.
func (s *braceScanner) inQuote(c byte) {
	switch {
	case s.escaped:
		s.escaped = false
	case c == '\\' && s.quote != '`':
		s.escaped = true
	case c == s.quote:
		s.quote = 0
	}
}

// bracket counts one byte of code.
func (s *braceScanner) bracket(c byte) {
	switch c {
	case '(':
		s.parens++
	case ')':
		s.parens--
	case '{':
		if !s.opened && s.parens == 0 && s.braces == 0 {
			s.opened = true
		}
		s.braces++
	case '}':
		s.braces--
		if s.opened && s.braces == 0 {
			s.closed = true
		}
	case ';':
		if !s.opened && s.parens == 0 && s.braces == 0 {
			s.bodiless = true
		}
	}
}
