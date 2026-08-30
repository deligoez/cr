package render

import "slices"

// Body is one author-facing body whose language Setting decides.
//
// It is a set rather than a single case because §8.1.1 only ever named one of
// them. Round 12's finding unspecified-render-language is the other: §8.4.3
// has cr write a review body carrying §4.5.4's disclosure — the active axes and
// every lens that did not run, with its reason — so that the honesty obligation
// reaches the author and not only the reviewer's terminal. A body written for
// the author in a language the author may not read reaches them no better than
// a terminal they never see, so it is written in Setting's language like every
// other author-facing body, and the set below says so where the renderer will
// read it rather than in a sentence the renderer's author may never see.
//
// The type is closed the way Lang is, and for the same reason: a body that is
// not one of the two below is unrepresentable, so a third author-facing body
// arrives here — where its language is already decided — instead of picking one
// wherever it happens to be written.
type Body struct{ name string }

// The author-facing bodies of v0.1.
var (
	// BodyComment is the per-record comment body of §8.1.1, which the agent
	// rewrites in Setting's language by editing `draft.md` per §8.1.2.
	BodyComment = Body{"comment"}
	// BodyReview is the review's own body of §8.4.3: §4.5.4's disclosure
	// above the payload hash of §8.3.3. It is not a line comment and is
	// unaffected by §1.6.1.
	BodyReview = Body{"review"}
)

// bodies is the closed set, in the order the spec reaches them: the comment
// body of §8.1 before the review body of §8.4.
var bodies = []Body{BodyComment, BodyReview}

// String returns the body's name.
func (b Body) String() string {
	return b.name
}

// AuthorFacing reports whether b is one of the bodies Setting's language
// governs. The zero Body is not.
func (b Body) AuthorFacing() bool {
	return slices.Contains(bodies, b)
}

// Bodies returns the set in spec order. The result is a copy, so a caller can
// neither widen it nor reorder it.
func Bodies() []Body {
	return append(make([]Body, 0, len(bodies)), bodies...)
}
