package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// execProgram reports the program an exec.Command or exec.CommandContext call
// starts, and whether the call starts one at all.
//
// A program that is not a plain string literal is reported as unfixed rather
// than passed over. The reader of a runner cannot say which command such a
// call runs, so neither can this guard, and the unanswerable case is the one
// that has to fail. An aliased import of os/exec fails the same way, from the
// other direction: the call is then invisible here and the runner it sits in
// goes missing from the result.
func execProgram(call *ast.CallExpr) (string, bool) {
	fun, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := fun.X.(*ast.Ident)
	if !ok || pkg.Name != "exec" {
		return "", false
	}
	// The program is the first argument of Command and the second of
	// CommandContext, which takes the context ahead of it.
	var at int
	switch fun.Sel.Name {
	case "Command":
		at = 0
	case "CommandContext":
		at = 1
	default:
		return "", false
	}
	if len(call.Args) <= at {
		return "", false
	}
	literal, ok := call.Args[at].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "a program named at run time", true
	}
	name, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "a program named at run time", true
	}
	return name, true
}

// §14.1 gives cr three runtime dependencies, and two of them are programs cr
// starts itself.
//
// TestCrReachesTheNetworkThroughOneRunnerAndNoOtherWay already fixes where a
// process may be started: os/exec is imported by the two files in runners and
// by nothing else. This fixes what those two start, which is a different
// claim that the first does not imply — an import scan reads
// exec.Command(helper, ...) exactly as it reads exec.Command("git", ...), so
// a fourth dependency introduced inside a runner passes it untouched.
//
// Requiring a literal costs the runners nothing, because each exists to drive
// exactly one command, and the result below says so: two runners, two
// invocations, one program each.
func TestTheRunnersStartNothingButGitAndGh(t *testing.T) {
	root := moduleRoot(t)

	var found []string
	for rel := range runners {
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, rel), nil, 0)
		require.NoError(t, err)

		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if program, isExec := execProgram(call); isExec {
				found = append(found, rel+" starts "+program)
			}
			return true
		})
	}

	slices.Sort(found)
	assert.Equal(t, []string{
		filepath.Join("internal", "gh", "run.go") + " starts gh",
		filepath.Join("internal", "git", "run.go") + " starts git",
	}, found, "§14.1: git and gh are the only programs cr starts, one to each runner")
}
