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

// reviewBodyText is one language's framing for the review body: the lines cr
// writes around §4.5.4's entries.
//
// The entries themselves are not in the table. Each is produced by the package
// that knows why its lens did not run — activation.Disabled, intent.Unavailable,
// reinvention.Unavailable, testadequacy.Unavailable, coverage.SkippedRole — and
// derives its own sentence from its own fields, which is what keeps what a
// reader is told and what a caller reads as data from drifting apart. Rewording
// them here would be a second answer to a question those packages already
// answered, and §4.5.4's reason is the half a reader acts on.
type reviewBodyText struct {
	lang Lang
	// heading opens the region, so the author can see whose block it is.
	heading string
	// axes introduces the active axes of §4.5.1.
	axes string
	// noAxes stands where the list would, when no axis ran at all.
	noAxes string
	// lenses introduces §4.5.4's entries.
	lenses string
	// noLenses stands where that list would, when every lens looked.
	noLenses string
}

// reviewBodyTexts is the built-in table, every language of langs written out.
//
// Written out rather than assembled, for the reason questionLabels is: §8.4.3
// makes this body cr's own, the author reads it as cr's statement about what was
// examined, and a composition a later caller can drive to a different result
// with a different argument is not a statement cr made.
var reviewBodyTexts = []reviewBodyText{
	{
		lang:     LangTR,
		heading:  "**cr — inceleme kapsamı**",
		axes:     "Bakılan eksenler:",
		noAxes:   "Bu turda hiçbir eksen çalışmadı.",
		lenses:   "Bakılmayan mercekler ve nedenleri:",
		noLenses: "Bakılmayan mercek yok.",
	},
	{
		lang:     LangEN,
		heading:  "**cr — review coverage**",
		axes:     "Axes reviewed:",
		noAxes:   "No axis ran this round.",
		lenses:   "Lenses that did not run, and why:",
		noLenses: "No lens was left unexamined.",
	},
}

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
// The body is not a line comment. §8.4.3 says so, and it is why nothing here
// takes an anchor: §1.6.1 anchors every posted comment to a line of the diff and
// §1.6.2's cap counts comments, neither of which this is.
func ReviewBody(
	lang Lang, active []string, disclosed []finding.HonestyDisclosure, hash string,
) (string, error) {
	text, known := reviewBodyOf(lang)
	if !known {
		return "", &UnknownLangError{Value: lang.String()}
	}
	lines := []string{text.heading, "", text.noAxes}
	if len(active) > 0 {
		lines[2] = text.axes + " " + strings.Join(active, ", ")
	}
	lines = append(lines, "", text.noLenses)
	if len(disclosed) > 0 {
		lines[len(lines)-1] = text.lenses
		for _, entry := range disclosed {
			lines = append(lines, "- "+entry.Disclosure())
		}
	}
	return strings.Join(lines, "\n") + regionSeparator + PayloadHashComment(hash), nil
}

// reviewBodyOf finds the built-in framing for lang, reporting whether the table
// has a row for it. The zero Lang has none, which is the case lang.go's
// enumeration exists to keep out of a renderer, so it is reported rather than
// answered with a default.
//
// The table is the review body's, so BodyReview is asked first whether
// Setting's language governs it, as QuestionLabel asks BodyComment.
func reviewBodyOf(lang Lang) (reviewBodyText, bool) {
	if !BodyReview.AuthorFacing() {
		return reviewBodyText{}, false
	}
	for _, text := range reviewBodyTexts {
		if text.lang == lang {
			return text, true
		}
	}
	return reviewBodyText{}, false
}
