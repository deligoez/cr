package post

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/text"
)

// Preimage is §8.3.3's explicit pre-image: the text the payload hash is taken
// over, built from the comments and from nothing else.
//
// §8.3.3 spells it out rather than hashing the request document because a JSON
// serialisation fixes neither field order nor spacing, and round 4's finding
// hash-over-undefined-serialisation is what that costs: §8.4.4 matches the
// embedded hash of an already-posted review to decide whether to adopt it or
// retry, so a value that shifts between binary versions turns the double-post
// guard into a silent duplicate post — which §8.4.4 itself calls the worse
// failure. Every choice below is therefore the spec's and not this file's.
//
// The review body is not in it. §8.4.3 embeds the hash in that body, so a
// pre-image including it could not be computed at all: the bytes to hash would
// have to contain their own digest. Excluding it is what makes the stored value
// and the embedded value the same value by construction.
//
// A comment with no `start_line` contributes 0, which is the field's value in
// the payload — GitHub tells a single-line comment from a multi-line one by the
// field's absence, and omitting it here would leave two comments at the same
// line contributing the same text.
func (r *Review) Preimage() string {
	// The pointers are sorted rather than the comments, so the payload
	// keeps the order §8.3.1 queued it in: §8.3.3 orders the pre-image,
	// not the review, and reordering the request would move a comment the
	// reviewer approved in one place into another.
	ordered := make([]*Comment, 0, len(r.Comments))
	for i := range r.Comments {
		ordered = append(ordered, &r.Comments[i])
	}
	slices.SortStableFunc(ordered, compareComments)
	texts := make([]string, 0, len(ordered))
	for _, comment := range ordered {
		texts = append(texts, comment.preimage())
	}
	return strings.Join(texts, "\n")
}

// Hash is the payload hash of §8.3.3: the normalised hash per §1.4 of the
// pre-image above.
//
// The error is §1.4 step 1's, reached when a body does not decode as UTF-8.
// text.NormalisedHash normalises inside rather than taking normalised text, so
// no caller can reach the digest with text that skipped a step.
func (r *Review) Hash() (string, error) {
	return text.NormalisedHash(r.Preimage())
}

// preimage is one comment's contribution: its path, start_line, line, side, and
// body joined by LF, in the order §8.3.3 names them.
func (c *Comment) preimage() string {
	return strings.Join([]string{
		c.Path,
		strconv.Itoa(c.StartLine),
		strconv.Itoa(c.Line),
		string(c.Side),
		c.Body,
	}, "\n")
}

// compareComments is §8.3.3's ordering: by path ascending, then start_line,
// then line, then side with LEFT before RIGHT, then record id by numeric
// suffix.
//
// Round 5's finding non-deterministic-ordering is why the order is total rather
// than nearly so. §6.4.1 deduplicates by (anchor.path, anchor.line, class), so
// two records of different classes at one anchor both survive and both post;
// with no tiebreak their relative order falls to whatever produced the slice,
// and the hash stops being a function of the payload.
//
// The side comparison is the two literals' own order — "LEFT" sorts before
// "RIGHT" — which is the direction §8.3.3 names, so no table is needed to say
// it. The record id falls back to the whole string when either is not the f<n>
// §6.1 spells: the ordering stays total for state cr did not write, rather than
// two unparseable ids tying and reopening round 5's finding under another name.
func compareComments(a, b *Comment) int {
	if order := strings.Compare(a.Path, b.Path); order != 0 {
		return order
	}
	if order := cmp.Compare(a.StartLine, b.StartLine); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Line, b.Line); order != 0 {
		return order
	}
	if order := strings.Compare(string(a.Side), string(b.Side)); order != 0 {
		return order
	}
	na, aParsed := finding.IDSuffix(a.Record)
	nb, bParsed := finding.IDSuffix(b.Record)
	if aParsed && bParsed {
		if order := cmp.Compare(na, nb); order != 0 {
			return order
		}
	}
	return strings.Compare(a.Record, b.Record)
}
