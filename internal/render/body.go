package render

import "slices"

// Body is one author-facing body whose language Setting decides.
//
// It is a closed set, and today it holds one member: the per-record comment
// body of §8.1.1. The review's own body of §8.4.3 is author-facing too, but it
// is not in the set — it is written in English whatever Setting says, per the
// user's 2026-09-13 decision, and ReviewBody takes no language at all. A body
// left out of the set here is how that decision is stated where the renderer
// reads it, rather than in a sentence the renderer's author may never see.
//
// The renderer reads it through AuthorFacing: every lookup of a built-in text
// keyed by language asks its body first, and a body outside the set has no text
// in any language. TestEveryLanguageKeyedTextIsReadThroughTheBodySet holds each
// such lookup to that, so a second language-governed body cannot be rendered
// without joining the set.
//
// The type is closed the way Lang is, and for the same reason: a body that is
// not one of those below is unrepresentable, so a new language-governed body
// arrives here — where its language is already decided — instead of picking one
// wherever it happens to be written.
type Body struct{ name string }

// The language-governed bodies of v0.1.
var (
	// BodyComment is the per-record comment body of §8.1.1, which the agent
	// rewrites in Setting's language by editing `draft.md` per §8.1.2.
	BodyComment = Body{"comment"}
)

// bodies is the closed set. Nothing outside this package can reach it, so no
// caller can widen it.
var bodies = []Body{BodyComment}

// AuthorFacing reports whether b is one of the bodies Setting's language
// governs. The zero Body is not.
func (b Body) AuthorFacing() bool {
	return slices.Contains(bodies, b)
}
