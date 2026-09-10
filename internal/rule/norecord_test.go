package rule

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.6.1.5 read off the source: nothing in this package makes a record.
//
// Detection reports hits and never verdicts, and a hit reaches a draft only as
// a record the agent writes. That holds only while no function here can mint
// one — a helper that turned a Hit into a finding.Finding, however convenient
// for a caller, would be a road from a detector to findings.ndjson that no
// agent walked, and the rule's own `kind` would ride it into §6.3's assertion
// register. So every place this package's own source names finding.Finding is
// read, and the three that could produce one are refused: a composite literal
// of the type, a `new` of it, and a function returning it in any shape.
//
// Naming the type as a parameter stays allowed, because that is what taking a
// record the agent already wrote looks like: Matcher.Suggest is handed one and
// offers it a suggestion, and cannot make the record it is handed.
func TestNothingInTheRulePackageMakesARecord(t *testing.T) {
	sources, err := os.ReadDir(".")
	require.NoError(t, err)

	named, making := 0, make([]string, 0)
	for _, source := range sources {
		name := source.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
		require.NoError(t, parseErr)
		named += len(recordTypesIn(parsed))
		making = append(making, recordsMadeIn(name, parsed)...)
	}

	require.NotZero(t, named,
		"the package names finding.Finding nowhere, so this guard measured nothing")
	assert.Empty(t, making,
		"§2.6.1.5: a hit reaches a draft only as a record the agent writes, and this package makes one")
}

// recordsMadeIn lists every construct in one file that could produce a record.
func recordsMadeIn(file string, parsed *ast.File) []string {
	making := make([]string, 0)
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch made := node.(type) {
		case *ast.CompositeLit:
			if len(recordTypesIn(made.Type)) > 0 {
				making = append(making, file+": a finding.Finding literal")
			}
		case *ast.CallExpr:
			if called, isIdent := made.Fun.(*ast.Ident); isIdent && called.Name == "new" &&
				len(made.Args) == 1 && len(recordTypesIn(made.Args[0])) > 0 {
				making = append(making, file+": new(finding.Finding)")
			}
		case *ast.FuncDecl:
			if made.Type.Results != nil && len(recordTypesIn(made.Type.Results)) > 0 {
				making = append(making, file+": "+made.Name.Name+" returns a finding.Finding")
			}
		}
		return true
	})
	return making
}

// recordTypesIn lists every `finding.Finding` named under one node.
func recordTypesIn(node ast.Node) []*ast.SelectorExpr {
	found := make([]*ast.SelectorExpr, 0)
	if node == nil {
		return found
	}
	ast.Inspect(node, func(inner ast.Node) bool {
		selector, isSelector := inner.(*ast.SelectorExpr)
		if !isSelector || selector.Sel.Name != "Finding" {
			return true
		}
		if pkg, isIdent := selector.X.(*ast.Ident); isIdent && pkg.Name == "finding" {
			found = append(found, selector)
		}
		return true
	})
	return found
}
