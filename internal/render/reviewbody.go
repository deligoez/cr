package render

import (
	"strings"

	"github.com/deligoez/cr/internal/finding"
)

// payloadHashOpen opens §8.4.3's HTML comment, the one cr writes into the
// review's own body so a round is identifiable after the fact.
//
// It is built on Reserved, like every other marker cr owns, which is what makes
// §8.1.3's refusal of a body carrying that sequence cover this one too: an
// agent body holding the string could otherwise counterfeit a round's identity,
// and §8.4.4 decides between adopting a posted review and posting again by
// reading it.
const payloadHashOpen = Reserved + "payload-hash "

// payloadHashClose ends it, spelled once beside the opening.
const payloadHashClose = " -->"

// PayloadHashComment is §8.4.3's HTML comment for one payload hash.
//
// It is a function rather than a format string at the call site because §8.4.4
// reads the line back: the writer and the reader have to agree exactly, and one
// spelling is how they agree.
func PayloadHashComment(hash string) string {
	return payloadHashOpen + hash + payloadHashClose
}

// PayloadHashIn recovers the payload hash §8.4.3 embedded in a review body,
// reporting whether the body carries one.
//
// It is the reader half of the pair above and lives beside it for the reason
// the writer does: §8.4.4 matches this value to decide between adopting a
// posted review and posting a second one, and a second reading of the marker
// written wherever that lands is how the two stop agreeing.
func PayloadHashIn(body string) (string, bool) {
	_, rest, found := strings.Cut(body, payloadHashOpen)
	if !found {
		return "", false
	}
	hash, _, closed := strings.Cut(rest, payloadHashClose)
	if !closed {
		return "", false
	}
	return hash, true
}

// The review body's framing: the lines cr writes around §4.5.4's entries.
//
// They are English whatever render.lang says, and so is every entry beneath
// them. render.lang governs the comment bodies of §8.1.1, which the agent
// rewrites per §8.1.2; the review body has no such path, and a body cr composed
// in a second language would be a second, built-in rendering of §4.5.4's
// reasons that nothing keeps in step with the English each producing package
// writes. The entries are produced by the package that knows why its lens did
// not run — activation.Disabled, intent.Unavailable, reinvention.Unavailable,
// testadequacy.Unavailable, coverage.SkippedRole — and are appended as those
// packages word them.
const (
	// reviewBodyHeading opens the region, so the author can see whose block
	// it is.
	reviewBodyHeading = "**cr — review coverage**"
	// reviewBodyAxes introduces the active axes of §4.5.1.
	reviewBodyAxes = "Axes reviewed:"
	// reviewBodyNoAxes stands where the list would, when no axis ran at all.
	reviewBodyNoAxes = "No axis ran this round."
	// reviewBodyLenses introduces §4.5.4's entries.
	reviewBodyLenses = "Lenses that did not run, and why:"
	// reviewBodyNoLenses stands where that list would, when every lens looked.
	reviewBodyNoLenses = "No lens was left unexamined."
)

// ReviewBody is §8.4.3's review body: §4.5.4's disclosure, and beneath it the
// payload hash of §8.3.3 as an HTML comment.
//
// The order is the requirement. §8.4.3 puts the disclosure *above* the hash
// comment, because the comment is for cr — §8.4.4 matches it to decide between
// adopting a posted review and posting again — and the disclosure is for the
// author. Round 8's finding posted-review-omits-honesty-disclosure is why the
// disclosure is here at all: §4.5.4 and §11.1 both stop at the reviewer's
// terminal, and a review arriving as line comments over an empty body reads as
// "I looked and this is all there was", which is the cheapest overstatement of
// certainty available.
//
// disclosed is every lens of §4.5.4 that did not run, which is what round 10's
// finding disclosure-scope-drift insisted on: not the disabled and unavailable
// axes alone, but the unavailable halves of §4.3.1 and §4.4.1 and a role skipped
// per §4.6.4 as well. It is taken as the collected interface rather than as each
// category, so a kind added to coverage.Lenses reaches the author without this
// function being touched — and so no caller can hand it three of the four.
//
// It takes no language: the body is English, per the framing above, so no
// render.lang value can change a byte of it.
//
// The body is not a line comment. §8.4.3 says so, and it is why nothing here
// takes an anchor: §1.6.1 anchors every posted comment to a line of the diff and
// §1.6.2's cap counts comments, neither of which this is.
func ReviewBody(active []string, disclosed []finding.HonestyDisclosure, hash string) string {
	lines := []string{reviewBodyHeading, "", reviewBodyNoAxes}
	if len(active) > 0 {
		lines[2] = reviewBodyAxes + " " + strings.Join(active, ", ")
	}
	lines = append(lines, "", reviewBodyNoLenses)
	if len(disclosed) > 0 {
		lines[len(lines)-1] = reviewBodyLenses
		for _, entry := range disclosed {
			lines = append(lines, "- "+entry.Disclosure())
		}
	}
	return strings.Join(lines, "\n") + regionSeparator + PayloadHashComment(hash)
}
