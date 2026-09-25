package render

import "slices"

// Body is one author-facing body whose language Setting decides.
//
// It is a closed set of two: the per-record comment body of §8.1.1, and since
// v0.14.0 the review's own body of §8.4.3, whose framing lines and axis names
// are built in per Setting as §8.1.4's labels are. Through v0.13.0 the review
// body was English whatever Setting said, per the user's 2026-09-13 decision,
// and stood outside the set; v0.14.0's §8.4.3 reverses that.
//
// The renderer reads it through AuthorFacing: every lookup of a built-in text
// keyed by language asks its body first, and a body outside the set has no text
// in any language. TestEveryLanguageKeyedTextIsReadThroughTheBodySet holds each
// such lookup to that, so a further language-governed body cannot be rendered
// without joining the set.
//
// The type is closed the way Lang is, and for the same reason: a body that is
// not one of those below is unrepresentable, so a new language-governed body
// arrives here — where its language is already decided — instead of picking one
// wherever it happens to be written.
type Body struct{ name string }

// The language-governed bodies.
var (
	// BodyComment is the per-record comment body of §8.1.1, which the agent
	// rewrites in Setting's language by editing `draft.md` per §8.1.2.
	BodyComment = Body{"comment"}
	// BodyReview is §8.4.3's review body, whose framing cr writes in
	// Setting's language and whose entries keep their lens's wording.
	BodyReview = Body{"review"}
)

// bodies is the closed set. Nothing outside this package can reach it, so no
// caller can widen it.
var bodies = []Body{BodyComment, BodyReview}

// AuthorFacing reports whether b is one of the bodies Setting's language
// governs. The zero Body is not.
func (b Body) AuthorFacing() bool {
	return slices.Contains(bodies, b)
}
