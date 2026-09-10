package cli

import (
	"go/ast"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.6.1.6's ledger has one writer, proved from the shape of the code.
//
// The ledger is not append-only in the byte sense: round 9's
// rule-stats-event-producer keys its entries and overwrites one on a second
// run, and `cr record` restates the round's dismissals. That makes a rewrite
// of the file an ordinary-looking operation, and the way §2.6.1.6 gets broken
// is a second writer that reads the file, forgets an event type, and publishes
// what it kept — §6.2.5 then stamps `origin: agent` on a citation of a hit that
// used to be there. So the file's one write door, state.UpdateRuleStats, is
// asserted to have one caller: rule's update, which hands the whole ledger to
// one merge that keeps every entry it does not own. The other half, what that
// merge keeps, is TestAWriteCarriesEveryEntryItDoesNotOwnThroughUnchanged.
func TestTheRuleLedgerHasOneWriter(t *testing.T) {
	doors := map[string]bool{"UpdateRuleStats": true}
	callers := map[string][]string{}
	eachSourceFile(t, func(rel string, file *ast.File) {
		for _, decl := range file.Decls {
			within, isFunc := decl.(*ast.FuncDecl)
			if !isFunc {
				continue
			}
			ast.Inspect(within, func(n ast.Node) bool {
				if call, isCall := n.(*ast.CallExpr); isCall && doors[calledName(call)] {
					callers[calledName(call)] = append(callers[calledName(call)],
						filepath.ToSlash(rel)+": "+within.Name.Name)
				}
				return true
			})
		}
	})

	require.Len(t, callers, len(doors), "a door to rule-stats.ndjson has no caller, so this guard measured nothing")
	for door := range doors {
		assert.Equal(t, []string{"internal/rule/stats.go: update"}, callers[door],
			"§2.6.1.6: rule-stats.ndjson has one writer, and %s is reached from elsewhere", door)
	}
}
