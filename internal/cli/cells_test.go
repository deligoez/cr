package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/state"
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

// The pull request the recording tests below run against.
const (
	cellsOwner = "acme"
	cellsRepo  = "api"
	cellsSlug  = cellsOwner + "/" + cellsRepo
	cellsPR    = 7
	cellsHead  = "be7e2c75aeb661ba3c96d7a7634f3f8e84bb7b91"
)

// briefedForCells writes the state a round leaves behind: two units, and two
// active roles, so the grid §10.2.2 counts is two by two and a recording that
// filled the wrong half of it has somewhere to be wrong.
//
// meta.json and units.ndjson are written directly rather than through
// `cr brief`, which would need a repository and a pull request; what these
// tests are about is what `cr cells record` does with a round, not how the
// round came to be. The unit hashes differ so §4.5.5's `unit_hash` cannot pass
// by carrying the same value everywhere.
func briefedForCells(t *testing.T) state.Layout {
	t.Helper()
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR(cellsOwner, cellsRepo, cellsPR))

	held, err := layout.LockPR(cellsOwner, cellsRepo, cellsPR)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: cellsOwner, Repo: cellsRepo, PR: cellsPR,
		IssueKey: "CR-7", Round: 1, Head: cellsHead,
		ActiveRoles: []string{"convention", "correctness"},
	}))
	require.NoError(t, held.Write(state.FileUnits,
		[]byte(`{"id":"u1","path":"src/Order.php","hash":"38372bc96eb4010e",`+
			`"head":"`+cellsHead+`","round":1}`+"\n"+
			`{"id":"u2","path":"src/Money.php","hash":"0a1b2c3d4e5f6071",`+
			`"head":"`+cellsHead+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// recordCells writes an NDJSON file outside the state tree and hands it to
// `cr cells record`.
func recordCells(t *testing.T, lines ...string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cells.ndjson")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	return runCLI(t, "cells", "record", strconv.Itoa(cellsPR), path, "--repo", cellsSlug)
}

// filledCells is coverage.ndjson as `(unit, role)` pairs, which is the shape
// §10.2.2 reads it in: the cells that are filled, and by omission the ones that
// are not.
func filledCells(t *testing.T, l state.Layout) []string {
	t.Helper()
	stored, err := state.ReadRecords[coverage.Cell](l, cellsOwner, cellsRepo, cellsPR, state.FileCoverage)
	require.NoError(t, err)
	at := make([]string, 0, len(stored))
	for i := range stored {
		at = append(at, stored[i].Unit+"/"+stored[i].Role)
	}
	return at
}

// Two of four cells are recorded, and the other two stay unfilled.
//
// This is §4.5.6's last clause read as behaviour: cr "MUST NOT invent a cell
// for a unit no role reported on — an unfilled cell is a coverage gap per
// §10.1.1, not something for cr to complete". Two units times two active roles
// is a grid of four, the file names two of them, and what the round holds
// afterwards is two. A cr that filled the rest with anything at all — a `pass`,
// an `na`, a placeholder — would be forming a judgement about code no role
// looked at, and §10.2.2 would count the round as complete on the strength of
// it.
//
// The two that are recorded sit at different units and different roles, so a
// writer that completed a row or a column has both to get wrong.
func TestRecordingTwoOfFourCellsLeavesTheOtherTwoUnfilled(t *testing.T) {
	layout := briefedForCells(t)

	require.NoError(t, recordCells(t,
		`{"unit":"u1","role":"correctness","result":"pass"}`,
		`{"unit":"u2","role":"convention","result":"question"}`))

	assert.Equal(t, []string{"u1/correctness", "u2/convention"}, filledCells(t, layout),
		"§4.5.6: the two cells no role reported on are a coverage gap, not cr's to complete")
}
