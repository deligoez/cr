package migrate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
)

// anchoredPath is the file every record in these tests was anchored in. The
// path is fixed rather than a parameter because no case here turns on it:
// §9.4.3's ordering is asserted by naming a second file in the head, not by
// anchoring elsewhere.
const anchoredPath = "order.go"

// anchored builds the anchor a record would hold over lines, with the window
// §9.2.3 records around them. The content hash is computed the way §9.2 says,
// so a test never writes a digest by hand.
func anchored(t *testing.T, before, lines, after []string) *finding.Anchor {
	t.Helper()
	hash, err := finding.AnchorContentHash(lines)
	require.NoError(t, err)
	return &finding.Anchor{
		Path: anchoredPath, Side: git.Right,
		StartLine: 1, Line: len(lines),
		ContentHash:   hash,
		ContextBefore: before,
		ContextAfter:  after,
	}
}

// §9.4.3: one run of lines carries the anchor's content hash, so the anchor
// moves to it — and the line reported is the run's last, which is what §9.2
// calls `line`.
//
// The code has moved down the file, which is the ordinary push: lines were
// added above it and nothing it says changed.
func TestAUniqueContentHashMovesTheAnchor(t *testing.T) {
	lines := []string{"func Discount(price int) int {", "\treturn price / 2"}
	anchor := anchored(t, nil, lines, nil)

	outcome := Anchor("f1", anchor, []File{{
		Path:  "order.go",
		Lines: append([]string{"package shop", "", "// added above"}, lines...),
	}})

	assert.True(t, outcome.Placed)
	assert.Equal(t, "order.go:5", outcome.To)
	assert.Equal(t, KeyContent, outcome.Key)
	assert.Equal(t, 1, outcome.Candidates)
}

// §9.4.4: the same lines appear twice, so the content hash alone settles
// nothing and the recorded window is what tells the two apart.
//
// This is the case the reference implementation declines outright and cr can
// answer, because §9.2.3 recorded a window and it is the only thing that
// distinguishes one copy of a boilerplate helper from another.
func TestTheRecordedWindowSeparatesTwoCopiesOfTheSameLines(t *testing.T) {
	lines := []string{"\tif err != nil {", "\t\treturn err", "\t}"}
	anchor := anchored(t, []string{"\terr := save(order)"}, lines, []string{"\treturn nil"})

	outcome := Anchor("f1", anchor, []File{{Path: "order.go", Lines: []string{
		"\terr := load(order)", // a first copy, under a different line
		"\tif err != nil {", "\t\treturn err", "\t}",
		"\terr := save(order)", // the second, under the one the record kept
		"\tif err != nil {", "\t\treturn err", "\t}",
		"\treturn nil",
	}}})

	assert.True(t, outcome.Placed)
	assert.Equal(t, "order.go:8", outcome.To, "the copy whose window matches")
	assert.Equal(t, KeyContext, outcome.Key)
}

// §9.4.4 and §9.4.6: two copies whose windows are identical too cannot be told
// apart, so the migration declines and the anchor is left where it was.
//
// This is the rule the release turns on. Placing either copy would be a comment
// on a line that merely resembles the recorded one — a wrong assertion carrying
// a confident location — and declining costs a question.
func TestTwoIndistinguishableCopiesDeclineRatherThanGuess(t *testing.T) {
	lines := []string{"\treturn errors.New(\"unreachable\")"}
	anchor := anchored(t, []string{"\t}"}, lines, []string{"}"})
	repeated := []string{"\t}", "\treturn errors.New(\"unreachable\")", "}"}

	outcome := Anchor("f1", anchor, []File{{
		Path:  "order.go",
		Lines: append(append([]string{}, repeated...), repeated...),
	}})

	assert.False(t, outcome.Placed, "§9.4.6: a migration that cannot be made unique is not a migration")
	assert.Empty(t, outcome.To)
	assert.Equal(t, KeyContext, outcome.Key, "the window was tried and did not settle it")
	assert.Equal(t, 2, outcome.Candidates, "the report says it was too many places, not nowhere")
	assert.Equal(t, "order.go:1", outcome.From, "the stored anchor is left as it was")
}

// A record whose code is gone declines with no candidates, and the report says
// so — "nowhere" and "too many places" are different problems for whoever reads
// it, so Candidates separates them.
func TestCodeThatIsGoneDeclinesWithNoCandidates(t *testing.T) {
	anchor := anchored(t, nil, []string{"\tlegacyDiscount(price)"}, nil)

	outcome := Anchor("f1", anchor, []File{{Path: "order.go", Lines: []string{"package shop"}}})

	assert.False(t, outcome.Placed)
	assert.Equal(t, 0, outcome.Candidates)
	assert.Equal(t, KeyContent, outcome.Key, "the content hash matched nothing, so the window was never reached")
}

// §9.4.3: the anchor's own file is searched first, so a record whose code was
// copied into another file keeps the one it named when both could carry it.
//
// The decision does not depend on the order — a unique candidate is unique
// wherever it sits — so what this pins is the report: when two files hold the
// same lines, the anchor declines rather than silently picking one, and the
// ordering only decides which file a reader is shown first.
func TestTheAnchorsOwnFileIsSearchedFirst(t *testing.T) {
	lines := []string{"\treturn price / 2"}
	anchor := anchored(t, nil, lines, nil)

	moved := Anchor("f1", anchor, []File{
		{Path: "copy.go", Lines: []string{"package shop"}},
		{Path: "order.go", Lines: lines},
	})
	assert.True(t, moved.Placed)
	assert.Equal(t, "order.go:1", moved.To)

	both := Anchor("f1", anchor, []File{
		{Path: "copy.go", Lines: lines},
		{Path: "order.go", Lines: lines},
	})
	assert.False(t, both.Placed, "the same lines in two files is §9.4.4's ambiguity across files")
	assert.Equal(t, 2, both.Candidates)
}

// §9.4.4: a candidate with fewer lines available on a side than the record
// recorded there does not survive the window.
//
// This is the rule the first implementation got backwards, and the test is
// written from the failure it produced. Letting an absent side count as
// agreement makes every candidate at a file boundary match every record: the
// candidate on line 1 below has nothing above it, the record recorded a line
// above, and the permissive comparison matched them and placed the anchor on
// the wrong copy. An absent side is not a matching side, so the boundary
// candidate is excluded and the real one is left alone and unique.
func TestACandidateWithLessWindowThanRecordedDoesNotSurvive(t *testing.T) {
	lines := []string{"\treturn price / 2"}
	anchor := anchored(t, []string{"func Discount(price int) int {"}, lines, nil)

	outcome := Anchor("f1", anchor, []File{{Path: "order.go", Lines: []string{
		"\treturn price / 2", // line 1: nothing above it at all
		"func Discount(price int) int {",
		"\treturn price / 2", // line 3: the window the record kept
	}}})

	assert.True(t, outcome.Placed)
	assert.Equal(t, "order.go:3", outcome.To,
		"the boundary candidate is excluded rather than matched vacuously")
	assert.Equal(t, KeyContext, outcome.Key)
}

// §1.4: the comparison is normalised, so a line that only changed its
// whitespace still matches.
//
// The content hash is already normalised per §9.2, so this is about the window:
// a reindented neighbour must not decline a migration, or every gofmt run would
// unplace every record in the file.
func TestTheWindowComparesUnderNormalisation(t *testing.T) {
	lines := []string{"\treturn price / 2"}
	anchor := anchored(t, []string{"func Discount(price int) int {   "}, lines, nil)

	outcome := Anchor("f1", anchor, []File{{Path: "order.go", Lines: []string{
		"\treturn price / 2", // a first copy with no window above it
		"func Discount(price int) int {",
		"\treturn price / 2",
	}}})

	assert.True(t, outcome.Placed)
	assert.Equal(t, "order.go:3", outcome.To)
}

// §9.2 admits no anchor whose span is empty or inverted, so meeting one means a
// stored record is malformed. It declines like any other unplaceable record
// rather than panicking on a negative length.
func TestAMalformedSpanDeclines(t *testing.T) {
	anchor := anchored(t, nil, []string{"\treturn price / 2"}, nil)
	anchor.StartLine, anchor.Line = 4, 2

	outcome := Anchor("f1", anchor, []File{{Path: "order.go", Lines: []string{"\treturn price / 2"}}})

	assert.False(t, outcome.Placed)
	assert.Equal(t, 0, outcome.Candidates)
}
