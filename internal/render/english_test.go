package render

import (
	"go/ast"
	"go/token"
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

// The only Turkish text in the tree is §8.1.4's question labels, per CLAUDE.md's
// English rule and the user's 2026-09-13 decision that §8.4.3's review body is
// English. So no string literal of this package's production code carries a
// Turkish letter unless it sits inside questionLabels.
//
// The scan checks its own instrument: it must find Turkish literals inside the
// label table, or a scan that recognises nothing would pass for a clean tree.
func TestNoTurkishTextOutsideTheQuestionLabels(t *testing.T) {
	inLabels, outside := 0, make([]string, 0)
	for _, file := range productionFiles(t) {
		var labels ast.Node
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Names) == 1 && vs.Names[0].Name == "questionLabels" {
					labels = vs
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
			if labels != nil && lit.Pos() >= labels.Pos() && lit.End() <= labels.End() {
				inLabels++
				return true
			}
			outside = append(outside, value)
			return true
		})
	}
	require.Positive(t, inLabels, "the scan recognises the Turkish question labels, so a clean result means something")
	assert.Empty(t, outside, "Turkish text in internal/render outside §8.1.4's question label table")
}
