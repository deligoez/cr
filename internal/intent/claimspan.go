package intent

import (
	"fmt"
	"strings"

	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/text"
)

// SpanTexts are the texts §3.3 validates a claim's span against.
//
// There are two, and §3.3 partitions the four sources of its table between
// them: the issue text of §3.1 for `description`, `acceptance` and `comment`,
// and the note the claim names for `note`. They travel together because the
// partition is made per claim and one file of claims may carry both kinds, not
// because any claim is ever checked against both.
type SpanTexts struct {
	// Issue is the issue text §3.1 read for this run, exactly as the
	// tracker command or `--intent-file` produced it. It is the text
	// §3.3.1's occurrence check searches, and the text §3.3's `issue_hash`
	// is taken over.
	Issue string
	// Notes is every note the §3.6 store holds for the issue key.
	//
	// It is the whole store rather than a selection, for the reason
	// note.Find's argument must be: §3.6.4 loads the notes by issue key and
	// §9.3.5 exempts the store from round scoping outright, so a slice
	// narrowed by round or by pull request would report a note somebody
	// recorded as one that does not exist and refuse a claim resting on it.
	Notes []note.Note
}

// spanSource is the one text one claim's span is checked against, together
// with the name the rejection calls it by and the rule the span is held to.
//
// It exists so that choosing the text and searching it are not the same step.
// The choice is made once, in against below, out of the claim's `source` and
// nothing else; every other line of this file is handed the result and cannot
// reach either text on its own. That is what §3.3.2's "each claim is checked
// against exactly one source, so the two rules never collide" asks for — not a
// convention that the note branch remembers not to look at the issue text, but
// a shape in which it has no issue text to look at.
type spanSource struct {
	// text is what the span is checked against.
	text string
	// name is what that text is called in a rejection, so the user is told
	// which of §3.3's two rules refused the claim.
	name string
	// whole is true when the span must be the whole of text rather than
	// occur somewhere inside it. §3.3.1 asks the second of a claim drawn
	// from the issue text; §3.3.2 asks the first of a claim drawn from a
	// note, whose span it sets to that note's body.
	whole bool
}

// holds reports whether span satisfies the rule this source was chosen under.
func (s spanSource) holds(span string) (bool, error) {
	if s.whole {
		return spanIsBody(s.text, span)
	}
	return SpanOccursIn(s.text, span), nil
}

// problem is what a rejection under this source's rule says is wrong.
func (s spanSource) problem() string {
	if s.whole {
		return fmt.Sprintf(
			"is not the body of %s; §3.3.2 sets the span of a claim drawn from a note to that note's body",
			s.name,
		)
	}
	return fmt.Sprintf(
		"does not occur in %s; §3.3 draws a claim from a verbatim substring of its source",
		s.name,
	)
}

// SpanOccursIn reports whether span occurs in source.
//
// It is one function because two sections ask the same question of the same
// pair. §3.3.1 rejects a claim whose span does not occur in the issue text, and
// §3.3.3 reports, for each claim, whether its span still occurs in the new
// issue text after drift. Two readings of "occurs" could disagree, and the
// disagreement would be silent in the worst direction: a claim recorded against
// one reading and reported stale against the other.
//
// The comparison is literal, because §3.3's table calls a span "the verbatim
// substring of the source text the claim was drawn from". Normalising first
// would accept a span that is not in the text, which is exactly the claim
// §3.3.1 exists to refuse.
func SpanOccursIn(source, span string) bool {
	return strings.Contains(source, span)
}

// spanIsBody reports whether span is the note body §3.3.2 sets it to.
//
// The two are compared normalised, per §1.4. §3.3.2 does not ask for a
// substring, so the verbatim-substring reason SpanOccursIn gives for a literal
// comparison does not reach this rule; what it asks is that the span be the
// body, and the one place a span is read as text afterwards is §3.3's
// `span_hash`, a normalised hash. Equal normalised forms are exactly the spans
// whose `span_hash` is the body's own, and any part of a note, or any note with
// a word added, normalises to something else.
func spanIsBody(body, span string) (bool, error) {
	normalBody, err := text.Normalise(body)
	if err != nil {
		return false, err
	}
	normalSpan, err := text.Normalise(span)
	if err != nil {
		return false, err
	}
	return normalBody == normalSpan, nil
}

// stillHolds is §3.3.3's per-claim question, asked of the texts as they now
// read and under the rule `cr claims record` held the claim to: a claim drawn
// from the issue text is asked whether its span still occurs there, and a claim
// drawn from a note whether its span is still the body of the note it names.
//
// A note the store no longer holds cannot hold the span, so that claim answers
// false. A retracted note still holds its body — §3.6.6 marks it rather than
// erasing it — and its consequence is §3.6.6's report, not this one.
func (s SpanTexts) stillHolds(claim *Claim) (bool, error) {
	if claim.Source != ClaimFromNote {
		return SpanOccursIn(s.Issue, claim.Span), nil
	}
	named, held := note.Find(s.Notes, claim.NoteID)
	if !held {
		return false, nil
	}
	return spanSource{text: named.Text, whole: true}.holds(claim.Span)
}

// span holds one claim to §3.3.1's occurrence rule or to §3.3.2's, whichever
// its `source` selects, and to exactly one of them.
func (c claimChecker) span(line int, claim *Claim) error {
	against, err := c.against(line, claim)
	if err != nil {
		return err
	}
	held, err := against.holds(claim.Span)
	if err != nil {
		return err
	}
	if !held {
		return &RejectedClaimError{
			File: c.file, Line: line, Field: "span",
			Problem: against.problem(),
		}
	}
	return nil
}

// against returns the one text §3.3 checks this claim's span in.
//
// The branch is `source` is `note` or it is not, which is the partition §3.3.1
// itself writes — "for a claim whose `source` is not `note`" — rather than a
// four-way switch over the table's values. That makes it total by
// construction: every source that exists and every source that could be added
// falls on one side or the other, so there is no claim this function can leave
// without a text and none it can hand two.
//
// A `note_id` naming a note the store does not hold is refused here rather than
// searched for in the issue text. §3.3.2 validates such a claim against the
// named note, and there is no such note: falling back to the issue text would
// be the one collision §3.3.2 rules out, and accepting the claim would admit a
// span nothing checked. §3.6.1 spells the id, so an id naming nothing is either
// a note that was never recorded or a store this run cannot see.
//
// A retracted note is not refused, and that is a reading of the spec rather
// than an oversight. §3.3.2 says the claim is validated against the named note
// and says nothing of its standing, and §3.6.6 gives retraction a different
// consequence entirely: a coverage cell or record citing a retracted note is
// *reported as needing re-evaluation in the next round*, which is a report and
// not a refusal, and which nothing can produce if the claim was never recorded.
// note.Standing is derived from the store every time it is asked for, so `cr
// draft` and `cr post` read the retraction as it stands when they run; a
// rejection here would move that decision to extraction time, where §3.6.6 does
// not put it.
func (c claimChecker) against(line int, claim *Claim) (spanSource, error) {
	if claim.Source != ClaimFromNote {
		return spanSource{text: c.spans.Issue, name: "the issue text"}, nil
	}
	named, held := note.Find(c.spans.Notes, claim.NoteID)
	if !held {
		return spanSource{}, &RejectedClaimError{
			File: c.file, Line: line, Field: "note_id",
			Problem: fmt.Sprintf(
				"names %q, and the context store for %s holds no such note; "+
					"§3.3.2 validates this claim against that note and against nothing else",
				claim.NoteID, c.issueKey,
			),
		}
	}
	return spanSource{text: named.Text, name: "note " + named.ID, whole: true}, nil
}
