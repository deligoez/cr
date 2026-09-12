package post

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
)

// knownPayloadHash is the payload hash of the three-comment round aRound builds.
//
// It is written out rather than computed in the test, because the test is about
// a value two binary versions must agree on: §8.4.4 matches this hash against an
// already-posted review to decide between adopting it and retrying, and a test
// that recomputed the expectation from the code under test would agree with
// every change to the pre-image. Recovering the value after a deliberate change
// to §8.3.3 means reading the new pre-image and writing the new digest here,
// which is the review the constant exists to force.
// The value was computed outside this tree — SHA-256 over the three comments'
// five fields joined by LF, truncated to sixteen hex characters — so the
// expectation is not the implementation restated.
const knownPayloadHash = "420012ebffc2b992"

// §8.3.3: the payload hash is a function of the payload, so the order the
// comments were queued in cannot change it.
//
// The reversal is the whole test. Round 5's finding non-deterministic-ordering
// is that two comments sharing a path and a line fall to slice order with no
// tiebreak, and a hash that moves with slice order defeats §8.4.4's double-post
// guard — the reconcile misses its match, clears the state, and the round is
// posted a second time.
func TestThePayloadHashIsFixedAndSurvivesReordering(t *testing.T) {
	records, bodies := aRound()
	review := Build(records, bodies)

	hash, err := review.Hash()
	require.NoError(t, err)
	assert.Equal(t, knownPayloadHash, hash,
		"§8.3.3's pre-image is explicit, so its hash is the same value on every binary")

	slices.Reverse(review.Comments)
	reordered, err := review.Hash()
	require.NoError(t, err)
	assert.Equal(t, hash, reordered,
		"§8.3.3 orders the pre-image itself, so the queueing order reaches no digest")
}

// §8.3.3 names the pre-image's five fields and their order, so the text is
// asserted rather than only its digest: a hash test alone passes on any
// pre-image the code happens to build.
func TestThePreimageIsTheFiveFieldsInSpecOrder(t *testing.T) {
	review := &Review{Comments: []Comment{{
		Record: "f1", Path: "app/Models/Order.php",
		StartLine: 40, Line: 42, Side: git.Right, Body: "Is the rounding deliberate?",
	}}}

	assert.Equal(t,
		"app/Models/Order.php\n40\n42\nRIGHT\nIs the rounding deliberate?",
		review.Preimage(),
		"§8.3.3: path, start_line, line, side, and body joined by LF, in that order")
}

// §8.3.3 has a single-line comment contribute its start_line like any other,
// and the payload's value for it is zero: GitHub tells the two kinds apart by
// the field's absence, and dropping the line here would leave a one-line
// comment and a multi-line one at the same anchor contributing the same text.
func TestASingleLineCommentContributesAZeroStartLine(t *testing.T) {
	review := &Review{Comments: []Comment{{
		Record: "f1", Path: "app/Models/Order.php", Line: 11, Side: git.Right, Body: "Dropped.",
	}}}

	assert.Equal(t, "app/Models/Order.php\n0\n11\nRIGHT\nDropped.", review.Preimage())
}

// §8.3.3's ordering is total, which round 5's finding non-deterministic-ordering
// is the argument for: every rung is exercised from a shuffled input, and the
// pre-image comes back in the one order the section names.
func TestTheOrderingRunsThroughEveryTiebreakSpec83NamesInTurn(t *testing.T) {
	shuffled := &Review{Comments: []Comment{
		{Record: "f10", Path: "b.go", Line: 3, Side: git.Right, Body: "j"},
		{Record: "f2", Path: "b.go", Line: 3, Side: git.Right, Body: "b"},
		{Record: "f3", Path: "b.go", Line: 3, Side: git.Left, Body: "l"},
		{Record: "f4", Path: "b.go", StartLine: 1, Line: 3, Side: git.Right, Body: "s"},
		{Record: "f5", Path: "a.go", Line: 9, Side: git.Right, Body: "p"},
	}}

	// Each comment contributes five lines, so every fifth is a body, and
	// the bodies in order are the comment order the section asks for.
	lines := strings.Split(shuffled.Preimage(), "\n")
	require.Len(t, lines, 5*len(shuffled.Comments))
	bodies := make([]string, 0, len(shuffled.Comments))
	for at := 4; at < len(lines); at += 5 {
		bodies = append(bodies, lines[at])
	}
	assert.Equal(t, []string{"p", "l", "b", "j", "s"}, bodies,
		"path ascending, then start_line, then line, then LEFT before RIGHT, then f<n> numerically")
}

// §8.3.3 excludes the review body from the pre-image, so §8.4.3's embedding of
// the hash into that body cannot change the value being embedded.
//
// Without the exclusion the requirement is not merely inconvenient, it is
// unconstructable — the bytes to hash would have to contain their own digest —
// which is what rounds 3's findings self-referential-payload-hash and
// self-referential-hash both reported.
func TestTheReviewBodyReachesNoDigest(t *testing.T) {
	records, bodies := aRound()
	review := Build(records, bodies)
	before, err := review.Hash()
	require.NoError(t, err)

	review.Body = "coverage disclosure\n\n<!-- cr:payload-hash " + before + " -->"
	after, err := review.Hash()
	require.NoError(t, err)

	assert.Equal(t, before, after,
		"§8.4.3 embeds the hash in the body, so the body cannot be part of what is hashed")
}
