package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cellType is the coverage cell's type name, which is what a construction of
// one has to spell whether it is written inside internal/coverage as `Cell` or
// outside it as `coverage.Cell`.
const cellType = "Cell"

// cellConstructions reports every expression in one file that would bring a
// coverage cell into being.
//
// Composite literals and `new` are the two ways a Go value comes into being
// without being decoded, and both are read off the AST rather than searched for
// as text: a type cannot be reached under an alias, built at run time, or
// spelled around.
func cellConstructions(file *ast.File) []string {
	var built []string
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CompositeLit:
			if namesCell(node.Type) {
				built = append(built, "builds a "+cellType+" literal")
			}
			// A literal inside a slice or map of cells may elide
			// its own type, so it builds a cell while naming
			// nothing: `[]*coverage.Cell{{…}}` is the shape, and a
			// detector reading types alone walks straight past it.
			if elementNamesCell(node.Type) {
				for _, elt := range node.Elts {
					inner, elided := elt.(*ast.CompositeLit)
					if elided && inner.Type == nil {
						built = append(built, "builds an elided "+cellType+" literal")
					}
				}
			}
		case *ast.CallExpr:
			fun, called := node.Fun.(*ast.Ident)
			if called && fun.Name == "new" && len(node.Args) == 1 && namesCell(node.Args[0]) {
				built = append(built, "allocates a "+cellType)
			}
		}
		return true
	})
	return built
}

// namesCell reports whether an expression names the coverage cell type, in
// either of the two spellings a Go file can use for it.
func namesCell(named ast.Expr) bool {
	switch typed := named.(type) {
	case *ast.StarExpr:
		return namesCell(typed.X)
	case *ast.Ident:
		return typed.Name == cellType
	case *ast.SelectorExpr:
		pkg, qualified := typed.X.(*ast.Ident)
		return qualified && pkg.Name == "coverage" && typed.Sel.Name == cellType
	}
	return false
}

// elementNamesCell reports whether a slice, array, or map type holds cells, so
// a literal that elided its own type can be attributed to one.
func elementNamesCell(named ast.Expr) bool {
	switch typed := named.(type) {
	case *ast.ArrayType:
		return namesCell(typed.Elt)
	case *ast.MapType:
		return namesCell(typed.Value)
	}
	return false
}

// Nothing in cr builds a coverage cell.
//
// §4.5.6 says cr "MUST NOT invent a cell for a unit no role reported on — an
// unfilled cell is a coverage gap per §10.1.1, not something for cr to
// complete", and this is that sentence made structural rather than tested by
// example. A test can only show that the cells cr wrote this time came from the
// agent's file; this shows there is no expression anywhere in cr that could
// produce a cell at all. Every Cell that exists was allocated by
// state.DecodeStamped out of a line an agent wrote, through the one generic
// call that names the type as a type argument and never as a value.
//
// The stakes are invariant 1 and P6 together. A cr-invented `pass` would be cr
// forming a judgement about code no role looked at, and §10.2.2 would count it
// towards a complete row — so "coverage is proven" would report the number of
// cells cr was willing to write rather than the number of lenses that looked. A
// gap is the honest answer, and §10.1.1 is where it belongs.
//
// The detector is exercised against a file that does construct one, because a
// guard that can convict nobody passes for the wrong reason: a renamed type or
// a mis-shaped AST match would leave the walk below silently empty.
func TestNothingBuildsACoverageCellOutsideItsDecoder(t *testing.T) {
	guilty, err := parser.ParseFile(token.NewFileSet(), "guilty.go", `package guilty

import "github.com/deligoez/cr/internal/coverage"

func invent() []*coverage.Cell {
	return []*coverage.Cell{{Unit: "u1", Role: "correctness", Result: "pass"}, new(coverage.Cell)}
}
`, parser.SkipObjectResolution)
	require.NoError(t, err)
	require.Len(t, cellConstructions(guilty), 2,
		"the detector must see both ways a cell can be constructed")

	var found []string
	eachSourceFile(t, func(rel string, file *ast.File) {
		for _, built := range cellConstructions(file) {
			found = append(found, rel+" "+built)
		}
	})

	assert.Empty(t, found,
		"§4.5.6: cr may not invent a cell, so nothing in cr may construct one")
}
