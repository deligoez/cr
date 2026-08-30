package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// namesOf renders a set of bodies as the names they go by, so the whole set can
// be asserted at once and in order.
func namesOf(set []Body) []string {
	names := make([]string, 0, len(set))
	for _, body := range set {
		names = append(names, body.String())
	}
	return names
}

// Round 12's finding unspecified-render-language: §8.4.3 has cr write a review
// body carrying §4.5.4's disclosure — the active axes and every lens that did
// not run, with its reason — expressly so the honesty obligation reaches the
// author rather than only the reviewer's terminal, and then names no language
// for it. §8.1.1 names one for the comment body alone.
//
// The answer is that there is nothing special about the review body: it is
// author-facing, so it is written in render.lang like the comment body. A body
// written for an author who cannot read it discloses no more than the terminal
// they never see.
//
// Asserted as a set rather than as a sentence, because the renderer is a later
// task and a sentence in a doc comment is not something its author has to pass.
func TestTheReviewBodyIsAnAuthorFacingBodyLikeTheComment(t *testing.T) {
	assert.Equal(t, []string{"comment", "review"}, namesOf(Bodies()),
		"the comment body of §8.1.1 and the review body of §8.4.3, in spec order")

	for _, body := range Bodies() {
		assert.Truef(t, body.AuthorFacing(), "%s is written in the configured language", body)
	}
	assert.True(t, BodyReview.AuthorFacing(),
		"§8.4.3's review body is rendered in render.lang like every other author-facing body")

	assert.False(t, Body{}.AuthorFacing(), "a body nothing named is no body")

	widened := Bodies()
	widened[0] = BodyReview
	assert.Equal(t, []string{"comment", "review"}, namesOf(Bodies()),
		"the set handed out is a copy: a caller can neither widen it nor reorder it")
}
