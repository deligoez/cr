package render

import (
	"go/ast"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// turkishLetters are the letters of the Turkish alphabet that English does not
// use. A string literal holding one is Turkish text, which is how the guard
// below recognises it without a dictionary.
const turkishLetters = "çğıİöşüÇĞÖŞÜ"

// builtInTables are the three tables whose Turkish rows §8.1.4, §8.4.3 and
// §8.1.7 build in per render.lang: the question labels, the review body's
// framing, and the evidence region's field names.
var builtInTables = []string{"questionLabels", "reviewFramings", "evidenceFieldNames"}

// The only Turkish text in the tree is §8.1.4's question labels, §8.4.3's
// review body framing and §8.1.7's evidence field names, per CLAUDE.md's
// English rule. So no string literal of
// this package's production code carries a Turkish letter unless it sits
// inside one of builtInTables.
//
// The scan checks its own instrument: it must find Turkish literals inside
// each table, or a scan that recognises nothing would pass for a clean tree.
func TestNoTurkishTextOutsideTheBuiltInTables(t *testing.T) {
	inTables, outside := make(map[string]int), make([]string, 0)
	for _, file := range productionFiles(t) {
		tables := make(map[string]ast.Node)
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Names) == 1 &&
					slices.Contains(builtInTables, vs.Names[0].Name) {
					tables[vs.Names[0].Name] = vs
				}
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			require.NoError(t, err)
			if !strings.ContainsAny(value, turkishLetters) {
				return true
			}
			for name, table := range tables {
				if lit.Pos() >= table.Pos() && lit.End() <= table.End() {
					inTables[name]++
					return true
				}
			}
			outside = append(outside, value)
			return true
		})
	}
	for _, name := range builtInTables {
		require.Positive(t, inTables[name], "the scan recognises %s's Turkish rows, so a clean result means something", name)
	}
	assert.Empty(t, outside, "Turkish text in internal/render outside §8.1.4's, §8.4.3's and §8.1.7's built-in tables")
}
