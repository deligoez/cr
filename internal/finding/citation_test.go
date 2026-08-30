package finding

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// citedLine is the line the fixed value below is taken over. It is indented
// with tabs, spaced twice around its operator, and left with trailing spaces,
// so §1.4's steps 3 and 4 all move something in it and its normal form —
// " if ($total > 0) {" — shares no byte run with what goes in. A stamp that
// reached the digest without normalising therefore cannot land on the same
// value by accident.
const citedLine = "\t\tif ($total  >  0) {   "

// citedLineHash is §1.4's normalised hash of that line, written out as a
// literal. An expected value computed the way the code computes it agrees with
// any implementation, including a wrong one; this one has to disagree with all
// but the right one. §6.2.3 stores the value so a v0.2 migration can compare
// against it, so a change that moves it is a change that makes every citation
// hash already in ~/.cr incomparable.
const citedLineHash = "0d6c58b99de69ac1"

// rawLineHash is the §1.4-shaped hash of citedLine as it stands: sixteen
// lowercase hex characters, stable, and wrong. It is what a stamp that skipped
// Normalise would store, and no assertion about the shape of a hash can tell it
// from the real one.
const rawLineHash = "65abc82d59be1121"

// §6.1 computes a citation's content hash and §6.2.3 stores it at record time,
// so the value is cr's own and a v0.2 drift check reads it back. That makes the
// value a contract rather than an artefact, and the three things worth fixing
// about it are the value itself, that it follows the cited line's content, and
// that it does not follow the line's whitespace — the last being what §1.4 is
// for and what a raw-bytes hash would break while passing everything else.
func TestTheStoredCitationHashIsFixedAndFollowsTheLine(t *testing.T) {
	assert.Equal(t, citedLineHash, stamped(t, citedLine),
		"§1.4's value for this line, fixed across every section that compares one")
	assert.NotEqual(t, rawLineHash, stamped(t, citedLine),
		"and not the hash of the raw line, which is equally well-formed and never comparable")

	assert.NotEqual(t, citedLineHash, stamped(t, "\t\tif ($total  >=  0) {   "),
		"the line changed, so the recorded hash must say so: this is the whole of what §6.2.3 reads it for")

	assert.Equal(t, citedLineHash, stamped(t, "    if ($total > 0) {"),
		"and the same code reindented is the same code, which is why §1.4 runs before the digest")
}

// stamped is the hash StampContentHash stores on a citation, with §1.4 step 1's
// error out of the way: every line here is valid UTF-8.
func stamped(t *testing.T, content string) string {
	t.Helper()
	citation := Citation{Path: "app/Models/Order.php", Line: 42}
	require.NoError(t, citation.StampContentHash(content))
	return citation.ContentHash
}

// theseLines are the shapes a citation hash and a one-line anchor hash could
// part company over if they were computed by two rules: indentation, trailing
// whitespace, whitespace that §1.4 leaves alone, an empty line, and a line that
// is nothing but whitespace and so normalises to the empty text.
var theseLines = []string{
	citedLine,
	"return $this->total;",
	"\treturn $this->total;",
	"return $this->total;  ",
	"return $this->total; ",
	"",
	"   \t ",
}

// §6.2.3 records the citation hash so a v0.2 migration can detect drift, and a
// drift check compares stored values: the citation's against the anchor's. That
// only means anything while both are the same rule, and round 8's finding is
// that nothing in the document said so.
//
// The values are asserted first, over the shapes two rules would disagree
// about. But a second rule that agrees today passes that and stops agreeing the
// moment either half is touched, and no assertion about values can tell the two
// apart — what separates them is only that a second rule exists at all, which
// nothing but the source can say. So the fence is read off the source as well:
// the stamp reaches a digest through AnchorContentHash and through nothing else.
func TestACitationHashesItsLineAsAOneLineAnchor(t *testing.T) {
	for _, line := range theseLines {
		assert.Equal(t, hashOf(t, []string{line}), stamped(t, line),
			"a citation names one line, so its hash is §9.2's rule over a range of one: %q", line)
	}

	assert.Equal(t, []string{"AnchorContentHash"}, calls(bodyOf(t, "citation.go", "StampContentHash")),
		"§9.2's rule is reached by calling it; a second call here is where the second rule starts")
}

// calls returns the name of every function a body calls, in source order, a
// selector spelled by its own last name so a package-qualified call is named as
// it is written.
func calls(body *ast.BlockStmt) []string {
	names := make([]string, 0, 1)
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			names = append(names, fn.Name)
		case *ast.SelectorExpr:
			names = append(names, fn.Sel.Name)
		}
		return true
	})
	return names
}
