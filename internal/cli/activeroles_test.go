package cli

import (
	"go/ast"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// activeRolesField is meta.json's `active_roles` under the name Go gives it, so
// the guard below searches for the identifier the compiler resolves rather than
// for the JSON key, which appears in a struct tag and in no assignment at all.
const activeRolesField = "ActiveRoles"

// writesActiveRoles are the paths allowed to set that field.
//
// internal/brief is `cr brief`, which §4.5.1 makes the writer: it establishes
// the axis decision and the resolved profile the definition rests on, and it
// already publishes meta.json. internal/state/meta.go is where the field is
// declared, and the two lines there normalise an absent list into an empty one
// per §12.3 rather than deciding anything.
var writesActiveRoles = []string{
	filepath.Join("internal", "brief") + string(filepath.Separator),
	filepath.Join("internal", "state", "meta.go"),
}

// One command settles meta.json's active-role list, and nothing else writes it.
//
// This is round 8's `circular-definition` finding held down from the other
// side. §4.5.1's definition is now non-circular — a role is active when its
// axis is active and its `profiles` list admits the resolved profile — but a
// definition with two writers is circular again in practice: §4.5.6 rejects a
// cell naming an inactive role and §10.2.2 counts a complete row of cells per
// active role, and the moment `cr review` or any later command could restate
// the field, those two would be reading whichever answer was written last.
//
// Assignments are what is fenced, not reads: every command is meant to read
// this field, and the whole point of one writer is that they all read one
// answer. A composite-literal key and an assignment to a selector are the two
// shapes a write takes; a method call named ActiveRoles is neither, which is
// why activation.Activation may carry the computation under the same name.
func TestOnlyBriefWritesTheActiveRoleList(t *testing.T) {
	var found []string
	eachSourceFile(t, func(rel string, file *ast.File) {
		for _, allowed := range writesActiveRoles {
			if strings.HasPrefix(rel, allowed) {
				return
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.KeyValueExpr:
				if key, named := node.Key.(*ast.Ident); named && key.Name == activeRolesField {
					found = append(found, rel+" sets "+activeRolesField+" in a composite literal")
				}
			case *ast.AssignStmt:
				for _, target := range node.Lhs {
					field, selected := target.(*ast.SelectorExpr)
					if selected && field.Sel.Name == activeRolesField {
						found = append(found, rel+" assigns "+activeRolesField)
					}
				}
			}
			return true
		})
	})

	assert.Empty(t, found,
		"§4.5.1: `cr brief` settles the active-role list, so nothing else may write it")

	// A guard that convicted nobody because the field had moved or been
	// renamed would pass and mean nothing, so the writer is required to
	// still be a writer.
	assert.Contains(t, string(repoFile(t, filepath.Join("internal", "brief", "store.go"))),
		activeRolesField+":",
		"the one allowed writer no longer sets the field, so this guard proves nothing")
}
