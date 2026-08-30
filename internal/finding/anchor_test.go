package finding

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
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

// Round 12's finding unordered-range-fields. §9.2 requires an anchor to carry
// both line numbers and says nothing about their order, so an anchor running
// backwards satisfies every word of the section — and the pair is read as a
// range downstream, by §6.2.2's probe-target containment and §8.3.3's payload
// ordering, neither of which re-checks it.
//
// Both boundaries are here because both are where the rule is decided rather
// than restated. §9.2's range is inclusive, so start_line == line is a one-line
// anchor and not an empty one, and the ordering test has to admit it; and line 1
// is a line while line 0 is what an absent field decodes to, which is why §9.2's
// required start_line is not GitHub's optional one.
func TestAnAnchorRangeRunsForwardsFromALineOfTheFile(t *testing.T) {
	for name, spans := range map[string][2]int{
		"a one-line anchor writes the same number twice": {12, 12},
		"a multi-line anchor runs forwards":              {12, 14},
		"the first line of the file is a line":           {1, 1},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, ValidateAnchor(state.FileFindings, 7, ranged(spans[0], spans[1])))
		})
	}

	for name, spans := range map[string][2]int{
		"an inverted range":                     {14, 12},
		"a range that ends one short of itself": {13, 12},
		"an absent start_line":                  {0, 14},
		"an absent line":                        {12, 0},
		"a start_line below the first line":     {-1, 14},
	} {
		t.Run(name, func(t *testing.T) {
			rejected := rejectsAnchor(t, ranged(spans[0], spans[1]))
			assert.Contains(t, rejected.Error(), "§9.2",
				"the user is told which section refused the range")
		})
	}
}

// anAnchor is a well-formed anchor, so each case above and below writes only the
// field it is about and a check the case never meant to trip cannot pass it.
func anAnchor() Anchor {
	return Anchor{
		Path:      "app/Models/User.php",
		Side:      git.Right,
		StartLine: 12,
		Line:      14,
	}
}

// ranged is anAnchor over one range.
func ranged(startLine, line int) *Anchor {
	anchor := anAnchor()
	anchor.StartLine, anchor.Line = startLine, line
	return &anchor
}

// rejectsAnchor asserts that §9.2 refuses an anchor, and that the refusal
// reaches the user as the record rejection §6.1.3 shapes: exit code 1 through
// RejectedRecordError, naming the line the record sits on and the field.
func rejectsAnchor(t *testing.T, anchor *Anchor) *RejectedRecordError {
	t.Helper()
	var rejected *RejectedRecordError
	require.ErrorAs(t, ValidateAnchor(state.FileFindings, 7, anchor), &rejected)
	assert.Equal(t, "anchor", rejected.Field)
	assert.Equal(t, 7, rejected.Line)
	assert.Equal(t, state.FileFindings, rejected.File)
	return rejected
}

// §9.2 closes the vocabulary in one sentence: valid side values are RIGHT and
// LEFT. It is not decoration. §9.2.1 and §6.1.2 resolve the two against
// different trees, so a third value names a tree cr resolves nothing against,
// and §6.2.2 turns on the same distinction — a probe target is always RIGHT, so
// a LEFT anchor can never reach the probed grade.
//
// The set has to be closed here rather than by the type: a side arrives on an
// agent's NDJSON line, and git.Side is a defined string, which any untyped
// constant in the tree is assignable to.
func TestAnAnchorNamesOneOfSection92sTwoSides(t *testing.T) {
	for _, side := range []git.Side{git.Right, git.Left} {
		anchor := anAnchor()
		anchor.Side = side
		assert.NoError(t, ValidateAnchor(state.FileFindings, 7, &anchor), side)
	}

	// The near misses are the ones worth naming: a side left out decodes to
	// the empty string, and the spellings GitHub's own API does not use are
	// what an agent writing from memory produces.
	for _, side := range []git.Side{"", "right", "Right", "RIGHT ", "MIDDLE", "BOTH"} {
		anchor := anAnchor()
		anchor.Side = side
		rejected := rejectsAnchor(t, &anchor)
		assert.Contains(t, rejected.Error(), string(git.Right))
		assert.Contains(t, rejected.Error(), string(git.Left))
		assert.Contains(t, rejected.Error(), string(side),
			"the value is quoted back, because the faults that reach here are invisible otherwise")
	}
}
