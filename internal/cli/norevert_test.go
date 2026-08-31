package cli

import (
	"go/ast"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The doors invariant 6 rests on: each writer that puts something into the
// sandbox for the length of a probe, and the wrapper that is the only way to
// reach it.
//
// There are two because the two obligations are opposites. §5.3.2 rewrites a
// file that is there and puts its content back; §5.4.2 creates a file that is
// not there and takes it away again. One door serving both would hold a single
// undo that has to decide which it is, and the decision would sit inside the
// one function invariant 6 exists to leave no room for a decision in.
var sandboxDoors = []struct {
	// writer is the unexported function that performs the write.
	writer string
	// wrapper is the exported function that arranges the undo.
	wrapper string
	// file is where the pair lives, as the one caller is reported.
	file string
	// undoes is what the wrapper takes back, for the failure message.
	undoes string
}{
	{
		writer:  "applySandboxMutation",
		wrapper: "UnderSandboxMutation",
		file:    "internal/state/mutate.go",
		undoes:  "§5.3.3's revert",
	},
	{
		writer:  "applySandboxTestFile",
		wrapper: "UnderSandboxTestFile",
		file:    "internal/state/probefile.go",
		undoes:  "§5.4.2's removal",
	},
}

// Invariant 6, proved from the shape of the code rather than from a run.
//
// "Probes revert. A mutation is undone even when the run fails, times out, or
// panics." §5.4.2 says the same of a gap probe's test file — "the removal MUST
// happen even when the run fails or times out" — so both doors are held to it.
// A test can show that the undo happened on the paths it exercises; it cannot
// show that the path someone adds next year also undoes. What can show that is
// the absence of a call site to get wrong, and this asserts that absence has
// three parts, for each door:
//
//  1. Nothing outside internal/state writes to the filesystem at all —
//     TestOnlyTheStatePackageWritesToTheFilesystem is that half, and it is what
//     makes this one worth stating: a future probe path cannot spell its own
//     write anywhere else, so it has to come through this package.
//  2. The writer is unexported and has exactly one caller. There is no exported
//     door onto it, so no caller outside internal/state can write at all, and
//     inside it there is one function that does.
//  3. That caller arranges the undo in a `defer` before it runs anything, and
//     the deferred call is the undo the write handed back. An undo written
//     after the run would be one `return err` away from being skipped, and no
//     test of the successful path would notice; a deferred one runs on the
//     error return, on the timeout, and on the panic alike.
//
// The one failure §5.3.3 and §5.4.2 name that none of this can answer is cr
// being killed outright, and §5.1.6 is what covers it: the check finds the
// unclean sandbox — a mutated tracked file, or a leftover artefact under the
// probe path template — on the next invocation and recreates it.
func TestOnlyTheRevertingWrappersTouchTheSandbox(t *testing.T) {
	callers := map[string][]string{}
	wrappers := map[string]*ast.FuncDecl{}
	eachSourceFile(t, func(rel string, file *ast.File) {
		for _, decl := range file.Decls {
			within, isFunc := decl.(*ast.FuncDecl)
			if !isFunc {
				continue
			}
			wrappers[within.Name.Name] = within
			ast.Inspect(within, func(n ast.Node) bool {
				call, isCall := n.(*ast.CallExpr)
				if !isCall {
					return true
				}
				if named, isSelector := call.Fun.(*ast.SelectorExpr); isSelector {
					callers[named.Sel.Name] = append(
						callers[named.Sel.Name], filepath.ToSlash(rel)+": "+within.Name.Name)
				}
				return true
			})
		}
	})

	for _, door := range sandboxDoors {
		t.Run(door.writer, func(t *testing.T) {
			reached := callers[door.writer]
			require.Len(t, reached, 1,
				"invariant 6: %s is reached from more than one place, so a write can happen "+
					"without %s that %s arranges", door.writer, door.undoes, door.wrapper)
			assert.Equal(t, door.file+": "+door.wrapper, reached[0],
				"invariant 6: the one caller of %s is the wrapper that undoes it", door.writer)

			wrapper := wrappers[door.wrapper]
			require.NotNil(t, wrapper,
				"%s is not in the tree, so this guard measured nothing", door.wrapper)
			body := wrapper.Body.List
			undo := boundTo(t, body, door.writer)
			deferred := deferIndex(t, body)
			assert.Less(t, deferred, len(body)-1,
				"invariant 6: %s defers nothing before it runs, so a failure returns past %s",
				door.wrapper, door.undoes)
			assert.True(t, calls(body[deferred], undo),
				"invariant 6: %s's deferred call does not run %s, which is the undo the write "+
					"handed back", door.wrapper, undo)
		})
	}
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
