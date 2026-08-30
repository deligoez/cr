package finding

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §9.2 writes one rule for both sizes: the content hash is the normalised hash
// per §1.4 of the lines from start_line to line inclusive, treated as one text,
// "so a single-line anchor and a multi-line anchor hash by the same rule".
//
// Asserting that the two calls return what strings.Join and text.NormalisedHash
// return would restate the implementation, so what is asserted here is what that
// sentence promises and a special-cased one-line path would quietly break:
// §1.4's normalisation reaches a one-line anchor exactly as it reaches a
// three-line one, the pre-image of several lines is those lines under one LF
// each — which is the text a one-line anchor over the same content carries — and
// the digest is taken over the whole range rather than its first line.
func TestAOneLineAndAThreeLineAnchorHashByTheSameFunction(t *testing.T) {
	one := hashOf(t, []string{"return $this->total;"})
	three := hashOf(t, []string{"if ($discount) {", "    $total -= $discount;", "}"})

	// §1.4 runs on both sizes. Trailing whitespace is what step 3 strips
	// and tabbed indentation is what step 4 collapses; a path that reached
	// the digest without normalising would show up here rather than as two
	// sites that agree round after round and never agree with each other.
	assert.Equal(t, one, hashOf(t, []string{"return $this->total;  "}))
	assert.Equal(t, three, hashOf(t, []string{"if ($discount) {", "\t$total -= $discount;", "}\t"}))

	// One text is one text however it was split on the way in, so the
	// three-line anchor hashes as the one-line anchor carrying the same LFs.
	assert.Equal(t, three, hashOf(t, []string{"if ($discount) {\n $total -= $discount;\n}"}))

	// The whole range reaches the digest. A hash of the first line alone,
	// or one blind to order, satisfies every assertion above.
	assert.NotEqual(t, three, hashOf(t, []string{"if ($discount) {"}))
	assert.NotEqual(t, three, hashOf(t, []string{"}", "    $total -= $discount;", "if ($discount) {"}))
}

// hashOf is one anchor's content hash, with §1.4 step 1's error out of the way:
// every pre-image here is valid UTF-8, so a failure is the test's own fault.
func hashOf(t *testing.T, lines []string) string {
	t.Helper()
	sum, err := AnchorContentHash(lines)
	require.NoError(t, err)
	return sum
}

// §9.2's "by the same rule" is a warning about one implementation in
// particular: a length test that hashes a single line directly and several
// joined. The two agree on the day they are written, so no assertion about
// values can tell them apart — the test above passes either way — and what
// separates them is only that a second rule exists at all, which nothing but the
// source can say.
//
// So the fence is read off the source: AnchorContentHash carries no branch of
// any kind. That is stronger than "no branch on the line count", deliberately. A
// branch on anything else is a second path to a value §7.4.1's waiver key and
// §6.1's citation hash have to be able to compare against this one, and a
// special case reachable from one of the three is the drift §6.2.3 says a v0.2
// migration would be reading.
func TestTheContentHashBranchesOnNothing(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "anchor.go", nil, parser.SkipObjectResolution)
	require.NoError(t, err)

	assert.False(t, branches(bodyOf(t, parsed, "AnchorContentHash")),
		"§9.2 gives a one-line and a multi-line anchor one rule, and a branch is where the second one starts")
	assert.True(t, branches(bodyOf(t, parsed, "ValidateAnchor")),
		"the neighbour that is all branches: without this the fence above would pass on a walker that matches nothing")
}

// branches reports whether a function body carries a branch of any kind.
func branches(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		switch node.(type) {
		case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt,
			*ast.ForStmt, *ast.RangeStmt:
			found = true
		}
		return !found
	})
	return found
}

// bodyOf returns the body of one function of a parsed file, and fails when the
// file declares no such name — a fence that lost its subject to a rename has
// stopped fencing, and would otherwise pass by inspecting nothing.
func bodyOf(t *testing.T, file *ast.File, name string) *ast.BlockStmt {
	t.Helper()
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn.Body
		}
	}
	require.FailNowf(t, "the fenced function is gone", "anchor.go declares no %s", name)
	return nil
}
