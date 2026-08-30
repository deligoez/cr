package finding

import (
	"errors"
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

// headHolding is a HeadFile over a fixed head: the files it holds, keyed by
// path and given as their lines. internal/git answers the same question against
// a real revision; what is exercised here is what §6.2.3 does with the answer.
func headHolding(files map[string][]string) HeadFile {
	return func(path string) ([]string, bool, error) {
		lines, held := files[path]
		return lines, held, nil
	}
}

// The file the citations below point into, whose second line is the one every
// hash in this file is taken over.
var orderModel = map[string][]string{
	"app/Models/Order.php":  {"<?php", citedLine, "return $this->total;"},
	"app/Support/Money.php": {"final class Money"},
}

// §6.2.3 resolves every entry in `citations` against the current head and
// stamps each one with the hash of the line it names.
//
// Every entry, and not the first that answers: §6.2.6 renders all of them into
// the draft block for the human to open. The first and last lines of a file are
// both cited, because they are the two the range check can be wrong about while
// looking right.
//
// The second entry arrives carrying a hash, which is the state a v0.2 drift
// check would read: a value taken at some earlier head. §6.2.3 has that hash do
// no validating work in v0.1, so resolution overwrites it from the line it just
// read and refuses nothing on account of it. A v0.1 that compared the two would
// pass this assertion only by accident and would reject a record §6.2.3 accepts.
func TestEveryCitationIsResolvedAndStampedFromTheHead(t *testing.T) {
	const takenAtAnEarlierHead = "0000000000000000"
	citations := []Citation{
		{Path: "app/Models/Order.php", Line: 2},
		{Path: "app/Models/Order.php", Line: 3, ContentHash: takenAtAnEarlierHead},
		{Path: "app/Support/Money.php", Line: 1},
	}

	require.NoError(t, ResolveCitations(headHolding(orderModel), "review-security.ndjson", 4, citations))

	assert.Equal(t, citedLineHash, citations[0].ContentHash,
		"§6.2.3 stores the hash of the line the citation names, computed by cr")
	assert.Equal(t, hashOf(t, []string{"return $this->total;"}), citations[1].ContentHash,
		"the last line of a file resolves, and a hash already on the entry is replaced rather than checked")
	assert.Equal(t, hashOf(t, []string{"final class Money"}), citations[2].ContentHash,
		"the first line of a second file resolves too, so no entry is passed over")
}

// §6.2.3 rejects with exit code 1 an entry whose path does not exist or whose
// line is out of range.
//
// Exit code 1 is §11.2's validation failure, and RejectedRecordError is the
// shape internal/cli maps onto it — the same shape §6.1.3's missing-field
// rejection takes, because the fault is the same kind: a file that read and
// parsed, carrying a record whose own content is wrong.
//
// The entry is named by its index. A record may carry several citations, and a
// rejection that named only the record would leave the author of the file to
// find which of them the head cannot open.
//
// Line 0 is a case of its own rather than a smaller version of line 4. Both
// trees number their lines from 1, so an entry that omits `line` decodes to
// zero and points at nothing, and a check written as a bound on the file's
// length alone would let it through.
func TestACitationTheHeadCannotOpenIsRejected(t *testing.T) {
	for name, unopenable := range map[string]struct {
		citations []Citation
		field     string
	}{
		"a path the head does not hold": {
			citations: []Citation{
				{Path: "app/Models/Order.php", Line: 1},
				{Path: "app/Models/Deleted.php", Line: 1},
			},
			field: "citations[1].path",
		},
		"a directory, which holds no line": {
			citations: []Citation{{Path: "app/Models", Line: 1}},
			field:     "citations[0].path",
		},
		"a line past the end of the file": {
			citations: []Citation{{Path: "app/Models/Order.php", Line: 4}},
			field:     "citations[0].line",
		},
		"a line before the first": {
			citations: []Citation{{Path: "app/Models/Order.php", Line: 0}},
			field:     "citations[0].line",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := ResolveCitations(headHolding(orderModel), "review-security.ndjson", 4, unopenable.citations)

			var rejected *RejectedRecordError
			require.ErrorAs(t, err, &rejected,
				"§6.2.3 rejects this entry, and §11.2 codes the rejection 1")
			assert.Equal(t, unopenable.field, rejected.Field,
				"a record carrying several citations must point at the one at fault")
			assert.Equal(t, "review-security.ndjson", rejected.File)
			assert.Equal(t, 4, rejected.Line)
		})
	}
}

// A head that cannot be read at all is not the record's fault.
//
// §6.2.3's rejection is about a citation pointing where the head holds nothing,
// which §11.2 codes 1. A git that refused is §3.1.3's failure and §11.2 codes it
// 3, so the error is passed through rather than turned into a record rejection:
// the two blame different people, and only one of them can fix it.
func TestAHeadThatCannotBeReadIsNotTheRecordsFault(t *testing.T) {
	refused := errors.New("git ls-tree: exit status 128")

	err := ResolveCitations(
		func(string) ([]string, bool, error) { return nil, false, refused },
		"review-security.ndjson", 4,
		[]Citation{{Path: "app/Models/Order.php", Line: 1}},
	)

	require.ErrorIs(t, err, refused)
	var rejected *RejectedRecordError
	assert.False(t, errors.As(err, &rejected),
		"§11.2 codes a refusing git 3; only the record's own content is a validation failure")
}
