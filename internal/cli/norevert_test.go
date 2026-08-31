package cli

import (
	"go/ast"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The names invariant 6 rests on: the writer that puts a mutation into the
// sandbox, and the wrapper that is the only way to reach it.
const (
	sandboxWriter  = "applySandboxMutation"
	revertingCall  = "UnderSandboxMutation"
	sandboxMutator = "internal/state/mutate.go"
)

// Invariant 6, proved from the shape of the code rather than from a run.
//
// "Probes revert. A mutation is undone even when the run fails, times out, or
// panics." A test can show that the revert happened on the paths it exercises;
// it cannot show that the path someone adds next year also reverts. What can
// show that is the absence of a call site to get wrong, and this asserts that
// absence has three parts:
//
//  1. Nothing outside internal/state writes to the filesystem at all —
//     TestOnlyTheStatePackageWritesToTheFilesystem is that half, and it is what
//     makes this one worth stating: a future mutation path cannot spell its own
//     write anywhere else, so it has to come through this package.
//  2. applySandboxMutation is unexported and has exactly one caller. There is
//     no exported door onto it, so no caller outside internal/state can apply a
//     mutation at all, and inside it there is one function that does.
//  3. That caller arranges the undo in a `defer` before it runs anything, and
//     the deferred call is the undo the apply handed back. A revert written
//     after the run would be one `return err` away from being skipped, and no
//     test of the successful path would notice; a deferred one runs on the
//     error return, on the timeout, and on the panic alike.
//
// The one failure §5.3.3 names that none of this can answer is cr being killed
// outright, and §5.3.3's own second sentence is what covers it: §5.1.6's check
// finds the unclean sandbox on the next invocation and recreates it.
func TestOnlyTheRevertingWrapperMutatesTheSandbox(t *testing.T) {
	var callers []string
	var wrapper *ast.FuncDecl
	eachSourceFile(t, func(rel string, file *ast.File) {
		for _, decl := range file.Decls {
			within, isFunc := decl.(*ast.FuncDecl)
			if !isFunc {
				continue
			}
			if within.Name.Name == revertingCall {
				wrapper = within
			}
			ast.Inspect(within, func(n ast.Node) bool {
				call, isCall := n.(*ast.CallExpr)
				if !isCall {
					return true
				}
				if named, isSelector := call.Fun.(*ast.SelectorExpr); isSelector &&
					named.Sel.Name == sandboxWriter {
					callers = append(callers, filepath.ToSlash(rel)+": "+within.Name.Name)
				}
				return true
			})
		}
	})

	require.Len(t, callers, 1,
		"invariant 6: %s is reached from more than one place, so a mutation can be applied "+
			"without the revert %s arranges", sandboxWriter, revertingCall)
	assert.Equal(t, sandboxMutator+": "+revertingCall, callers[0],
		"invariant 6: the one caller of %s is the wrapper that reverts", sandboxWriter)

	require.NotNil(t, wrapper, "%s is not in the tree, so this guard measured nothing", revertingCall)
	body := wrapper.Body.List
	undo := boundTo(t, body, sandboxWriter)
	deferred := deferIndex(t, body)
	assert.Less(t, deferred, len(body)-1,
		"invariant 6: %s defers nothing before it runs, so a failure returns past the revert",
		revertingCall)
	assert.True(t, calls(body[deferred], undo),
		"invariant 6: %s's deferred call does not run %s, which is the undo the apply handed back",
		revertingCall, undo)
}

// boundTo is the name the wrapper binds the writer's undo to, which is what the
// deferred call has to run.
func boundTo(t *testing.T, body []ast.Stmt, writer string) string {
	t.Helper()
	for _, stmt := range body {
		assigned, isAssign := stmt.(*ast.AssignStmt)
		if !isAssign || len(assigned.Lhs) == 0 {
			continue
		}
		for _, value := range assigned.Rhs {
			call, isCall := value.(*ast.CallExpr)
			if !isCall {
				continue
			}
			if named, isSelector := call.Fun.(*ast.SelectorExpr); isSelector &&
				named.Sel.Name == writer {
				name, isIdent := assigned.Lhs[0].(*ast.Ident)
				require.True(t, isIdent, "%s's result is not bound to a name", writer)
				return name.Name
			}
		}
	}
	t.Fatalf("%s is not called in the wrapper that is meant to be its only caller", writer)
	return ""
}

// deferIndex is where the wrapper's deferred statement sits in its body.
func deferIndex(t *testing.T, body []ast.Stmt) int {
	t.Helper()
	for i, stmt := range body {
		if _, isDefer := stmt.(*ast.DeferStmt); isDefer {
			return i
		}
	}
	t.Fatal("invariant 6: the wrapper defers nothing, so nothing runs on the panic path")
	return -1
}

// calls reports whether a statement runs the named function anywhere inside it.
func calls(stmt ast.Stmt, name string) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if named, isIdent := call.Fun.(*ast.Ident); isIdent && named.Name == name {
			found = true
		}
		return true
	})
	return found
}
