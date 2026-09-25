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

// reviewFraming is the review body's framing in one language: the lines cr
// writes around §4.5.4's entries, and the name each axis goes by.
type reviewFraming struct {
	// lang is the language the row is written in.
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
	// names are the axis names by §1.5's axis id, spelled out rather than
	// taken from internal/axis, whose tests import this package through
	// internal/config; an id absent here goes by itself.
	names map[string]string
}

// reviewFramings is §8.4.3's framing for every language §8.1.1 admits, built in
// and not configurable, as §8.1.4's labels are.
//
// Only the framing and the axis names are per language. Each entry beneath is
// produced by the package that knows why its lens did not run —
// activation.Disabled, intent.Unavailable, reinvention.Unavailable,
// testadequacy.Unavailable, coverage.SkippedRole — and keeps the wording that
// package gives it for the author, per AuthorDisclosure; a second, built-in
// rendering of those reasons would be one nothing keeps in step with them.
//
// Through cr 0.13.0 the body was English under every render.lang, so a Turkish
// team's author received it as the one English text of the review.
var reviewFramings = []reviewFraming{
	{
		lang:     LangEN,
		heading:  "**cr — review coverage**",
		axes:     "Axes reviewed:",
		noAxes:   "No axis ran this round.",
		lenses:   "Lenses that did not run, and why:",
		noLenses: "No lens was left unexamined.",
	},
	{
		lang:     LangTR,
		heading:  "**cr — inceleme kapsamı**",
		axes:     "İncelenen eksenler:",
		noAxes:   "Bu turda hiçbir eksen çalışmadı.",
		lenses:   "Çalışmayan incelemeler ve nedenleri:",
		noLenses: "Hiçbir inceleme atlanmadı.",
		names: map[string]string{
			"intent": "niyet", "correctness": "doğruluk", "convention": "kod kuralları", "test": "test",
		},
	},
}

// framingIn is the review body's framing row for lang, and the zero row when
// BodyReview is not a body Setting's language governs or lang has no row: a
// body outside the set has text in no language.
func framingIn(lang Lang) reviewFraming {
	if !BodyReview.AuthorFacing() {
		return reviewFraming{}
	}
	for _, row := range reviewFramings {
		if row.lang == lang {
			return row
		}
	}
	return reviewFraming{}
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
// lang is render.lang, which chooses the framing and the axis names and
// nothing else, per reviewFramings.
//
// The body is not a line comment. §8.4.3 says so, and it is why nothing here
// takes an anchor: §1.6.1 anchors every posted comment to a line of the diff and
// §1.6.2's cap counts comments, neither of which this is.
func ReviewBody(lang Lang, active []string, disclosed []finding.HonestyDisclosure, hash string) string {
	framing := framingIn(lang)
	lines := []string{framing.heading, "", framing.noAxes}
	if len(active) > 0 {
		named := make([]string, 0, len(active))
		for _, id := range active {
			if name, found := framing.names[id]; found {
				id = name
			}
			named = append(named, id)
		}
		lines[2] = framing.axes + " " + strings.Join(named, ", ")
	}
	lines = append(lines, "", framing.noLenses)
	if len(disclosed) > 0 {
		lines[len(lines)-1] = framing.lenses
		for _, entry := range disclosed {
			lines = append(lines, "- "+forTheAuthor(entry))
		}
	}
	return strings.Join(lines, "\n") + regionSeparator + PayloadHashComment(hash)
}

// AuthorDisclosure is a §4.5.4 entry that also words itself for the pull
// request's author: which lens did not run, which files it did not look at, and
// what it therefore did not check, with no configuration or command to change
// and no section of the spec to read. Disclosure is worded for the reviewer, who
// can act on the reason; the author cannot, and a body telling them to add a
// glob to a profile is cr's operator talking to the wrong person.
//
// The sentence is produced where Disclosure is, by the package that knows why
// its lens did not run, so the two cannot describe different causes.
type AuthorDisclosure interface {
	AuthorDisclosure() string
}

// forTheAuthor is one entry of the review body: its author wording, and its
// reviewer wording only for a kind that carries none, since saying a lens did not
// run in the reviewer's words is better than not saying it.
func forTheAuthor(entry finding.HonestyDisclosure) string {
	if worded, ok := entry.(AuthorDisclosure); ok {
		return worded.AuthorDisclosure()
	}
	return entry.Disclosure()
}
