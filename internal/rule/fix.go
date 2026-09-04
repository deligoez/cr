package rule

import (
	"fmt"
	"regexp"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
)

// Replacement is §2.6.2.1's generation: `fix.replace` applied to the line a
// hit matched, with `fix.with` as the replacement, producing suggestion text.
// The second result says whether a suggestion was produced at all.
//
// Text is all that is produced. §2.6.2.3 is the invariant behind the signature:
// cr never applies a fix to any file, so there is no path here, no file is
// opened, and nothing is written — the caller is handed a string and decides
// what to do with it, exactly as §2.6.2.3's "suggestion text for the author to
// accept" describes.
//
// Two cases produce nothing rather than producing something empty or unchanged.
// A rule with no `fix` block suggests nothing, which is why Fix is nil-able for
// the reason Detect is. And a `fix.replace` that does not match the line
// suggests nothing either: a replacement Go's regexp package would leave the
// line untouched is a suggestion asking the author to accept the code they
// already wrote, and §1.6's economy pays for that comment in the same trust as
// a wrong one. The identity check is the second half of the same reading, for a
// pattern that matched and replaced itself.
//
// The replacement is expanded, not pasted. `$1` in `fix.with` takes the value
// `fix.replace` captured, which is what makes a fix able to rewrite a line
// rather than only to overwrite it, and it is Go's own `Regexp.ReplaceAllString`
// doing it rather than a substitution invented here.
func (m *Matcher) Replacement(hit *Hit) (string, bool) {
	if m.Fix == nil || !m.Fix.MatchString(hit.Text) {
		return "", false
	}
	text := m.Fix.ReplaceAllString(hit.Text, m.Rule.Fix.With)
	if text == hit.Text {
		return "", false
	}
	return text, true
}

// Suggest offers the suggestion generated for one hit to the record written
// from it, and reports whether the suggestion survived. It is §2.6.2.2: the
// generated suggestion passes §8.2's validation before drafting, and one that
// fails is dropped.
//
// What is dropped is the suggestion and never the record. The two are separable
// because they say different things: the record says a rule's standard was
// broken at a place, and the suggestion says what to write instead. cr can
// establish the first from its own detection, and §2.6.2.4 says plainly it
// cannot establish the second compiles, parses, or preserves behaviour. So a
// suggestion cr cannot place is dropped and the finding it belongs to still
// reaches the author — the opposite arrangement would let an unusable
// replacement silence a violation cr actually found.
//
// The record is left untouched on the failing path rather than cleared. A
// record arriving with a suggestion of its own carries `suggestion_origin:
// agent` under §6.1's table, and clearing the field here would delete the
// agent's own work in the name of a rule that produced nothing usable.
func (m *Matcher) Suggest(record *finding.Finding, hit *Hit, hunks []git.Hunk) bool {
	text, produced := m.Replacement(hit)
	if !produced || !placeable(&record.Anchor, hunks) {
		return false
	}
	record.Suggestion = text
	return true
}

// placeable reports whether §8.2 admits a suggestion replacing the record's
// anchored range: §8.2.1's contiguous range existing in the diff on the RIGHT
// side, and §8.2.2's containment within one hunk.
//
// It reads the record's anchor rather than the hit's line, because the anchored
// range is what a posted suggestion replaces — GitHub attaches a suggestion to
// the comment's own range, so a one-line fix on a record anchored across four
// lines would replace all four. Validating the line the fix was computed from
// would answer a question nobody asks.
//
// The RIGHT-side test is asked of the anchor and of the hunk both. §9.2 numbers
// a LEFT anchor in the merge base, where a replacement would rewrite a line the
// change already removed; and a hunk that adds no line is numbered on LEFT with
// a head range that is an insertion point rather than a line, so a suggestion
// landing in it would replace nothing that exists at the head.
func placeable(anchor *finding.Anchor, hunks []git.Hunk) bool {
	if anchor.Side != git.Right || anchor.StartLine < 1 || anchor.Line < anchor.StartLine {
		return false
	}
	for at := range hunks {
		hunk := &hunks[at]
		if hunk.Path != anchor.Path || hunk.Side != git.Right {
			continue
		}
		start, end := hunk.HeadRange()
		if anchor.StartLine >= start && anchor.Line <= end {
			return true
		}
	}
	return false
}

// compileFix holds one `fix` block to §2.6.2.1: `fix.replace` is a regular
// expression, in the Go `regexp` syntax §2.6.1.2 fixes for the other pattern a
// rule can carry. A rule with no `fix` block compiles to no pattern, which is
// how Replacement tells suggesting nothing from suggesting the empty string.
//
// Every rule of the corpus is asked, including one with no `detect` block that
// can never produce a hit for the fix to rewrite. §2.6 item 5 makes the file
// malformed, not the mechanism unusable, so a fix that cannot compile is
// reported whether or not this run had a line to apply it to — the alternative
// hides the fault until the day someone adds a detector to the same file.
//
// A `fix.replace` cr cannot compile makes the file malformed under §2.6 item 5,
// with the same exit code and the same shape of message as `detect.pattern`,
// and for the same reason compile gives there: its author wrote it to be run,
// and a corpus that dropped it would report full coverage over a rule it never
// applied. Reporting it as malformed is the only reading that does not decide
// on the author's behalf that the fix did not matter.
func (r *Resolved) compileFix() (*regexp.Regexp, error) {
	if r.Rule.Fix == nil {
		return nil, nil
	}
	pattern, err := regexp.Compile(r.Rule.Fix.Replace)
	if err != nil {
		return nil, &MalformedError{
			File:  r.Path,
			Field: "fix.replace",
			Problem: fmt.Sprintf(
				"of rule %q is not a Go regexp: %v",
				r.Rule.ID, err,
			),
		}
	}
	return pattern, nil
}
