package symbol

import "regexp"

// identifier finds every identifier on a line, spelled the way the scanners'
// rules spell a declared name, so a name the index holds is found as a
// reference in exactly the spelling it was declared in.
var identifier = regexp.MustCompile(ident)

// Referenced names the head symbols the file at path refers to, given the lines
// the head holds there: every identifier on those lines that some declaration of
// the index is named, in the order the file first names it, once each.
//
// It is §4.4.1's "the symbols they reference" answered out of §4.3.1's index,
// and it is a match of names rather than a resolution. A test calling `Load`
// references every `Load` the head declares, in whichever file; which of them
// the test exercises, and whether that covers the unit, is the classification
// §2.1.3 reserves for the agent. So a name two files declare is attached once,
// and the agent is shown the name rather than a choice cr made between them.
//
// Two occurrences are not references, for the reasons scan skips them. A line
// that begins a comment is not code, and a declaration's own name on its own
// line in this file is the file declaring the symbol, not referring to it — a
// test file's `func TestLoad(` would otherwise reference itself.
//
// An identifier inside a string literal is still read. A spurious reference
// costs the agent one name it looks at and sets aside, which is evidence
// supplied rather than an assertion made; recognising every language's string
// syntax is the parser symbol.go declines to be.
func (i *Index) Referenced(path string, lines []string) []string {
	declared := make(map[string]bool, len(i.Decls))
	sites := make(map[Decl]bool)
	for _, decl := range i.Decls {
		declared[decl.Name] = true
		if decl.Path == path {
			sites[Decl{Path: path, Line: decl.Line, Name: decl.Name}] = true
		}
	}
	named := make([]string, 0)
	seen := make(map[string]bool)
	for at, text := range lines {
		if commented(text) {
			continue
		}
		for _, name := range identifier.FindAllString(text, -1) {
			if !declared[name] || seen[name] || sites[Decl{Path: path, Line: at + 1, Name: name}] {
				continue
			}
			seen[name] = true
			named = append(named, name)
		}
	}
	return named
}
