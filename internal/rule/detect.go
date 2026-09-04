package rule

import (
	"regexp"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/glob"
)

// Matcher is one rule paired with the compiled form of its `detect.pattern`.
//
// The pair is the seam between §2.6.1.2 and §2.6.1.1. Compiling the pattern is
// the first one's obligation — Go `regexp` syntax, `mode` closed at `regex`,
// and an abort naming the rule when it does not compile — and evaluating it is
// this file's. Keeping them apart means detection cannot silently skip a rule
// whose pattern was unusable: it never sees one, because a corpus that could
// not be compiled never became a Matcher at all.
type Matcher struct {
	// Rule is the rule the pattern came from, whole, because a hit carries
	// the id (§2.6 item 3) and a record quotes the rationale (item 4).
	Rule Rule
	// Pattern is `detect.pattern` compiled.
	Pattern *regexp.Regexp
}

// Hit is one mechanical match of a rule's `detect` block against one changed
// line, and nothing more.
//
// §2.6.1.5 is what fixes its size: detection reports hits, never verdicts. So a
// Hit carries no severity, no kind and no grade — the rule's own defaults are
// on the Rule, and whether this hit ever becomes a record is the agent's call
// under §2.6.1.5, which drops an unconfirmed one rather than posting it. What
// it does carry is what §2.6.1.3 needs to write a `citations` entry and what
// §2.6.1.6 needs to append to `rule-stats.ndjson`: the rule id, the matched
// path, and the matched line.
type Hit struct {
	// RuleID is the rule whose pattern matched, per §2.6 item 3.
	RuleID string
	// Path is the file the matched line is in, repository-relative.
	Path string
	// Line is the matched line's number at the head. §2.6.1.1 evaluates
	// only RIGHT-side lines, so a hit is always numbered on §9.2's RIGHT
	// and never carries a side that could be recorded as the other one.
	Line int
	// Text is the matched line's content, which §2.6.2.1 applies
	// `fix.replace` to.
	Text string
}

// Evaluate applies every matcher over the diff and returns the hits, in corpus
// order and then in diff order. It is §2.6.1.1's "evaluated mechanically by
// cr", and it is named apart from the Detect block it evaluates because the two
// are a rule's declaration and cr's reading of it.
//
// §2.6.1.1 fixes the scope twice over, and the signature is the first half:
// hunks are the only thing Evaluate can read. There is no repository root here,
// no file is opened, and nothing is resolved against the head — so a pattern
// matching a thousand unchanged lines of the repository produces nothing,
// because those lines never reach this function. A reviewer who comments on
// code the author did not touch has spent trust on a line that was not up for
// review, and §1.6's economy has no way to earn it back.
//
// The second half is the RIGHT-side filter. A unified diff has no marker for a
// modification — an edited line is a removal and an addition standing together
// — so §3.4.1's added RIGHT-side lines are exactly the added and modified lines
// §2.6.1.1 admits, and internal/git already partitions them that way. A hunk
// that adds nothing carries its removed LEFT-side lines instead, and those are
// skipped: a rule enforces how code must be written, and a line the change
// deleted is no longer written anywhere.
func Evaluate(matchers []Matcher, hunks []git.Hunk) []Hit {
	hits := make([]Hit, 0)
	for m := range matchers {
		matcher := &matchers[m]
		for h := range hunks {
			hunk := &hunks[h]
			if !matcher.Rule.Applies(hunk.Path) {
				continue
			}
			hits = matcher.appendHits(hits, hunk)
		}
	}
	return hits
}

// appendHits adds one matcher's hits within one hunk, in ascending line order.
func (m *Matcher) appendHits(hits []Hit, hunk *git.Hunk) []Hit {
	for _, line := range hunk.Changed {
		if line.Side != git.Right || !m.Pattern.MatchString(line.Text) {
			continue
		}
		hits = append(hits, Hit{
			RuleID: m.Rule.ID,
			Path:   hunk.Path,
			Line:   line.Line,
			Text:   line.Text,
		})
	}
	return hits
}

// Applies reports whether the rule applies to one repository-relative path, per
// §2.6's `globs` and `exempt` rows.
//
// The two are asked in that order because that is what "excluded" means: an
// `exempt` entry subtracts from whatever `globs` selected, so a legacy area
// stays out even when it sits inside the rule's own scope. Reversing them would
// make `exempt` a suggestion — a directory named by both rows would be enforced
// against, which is the one arrangement its author was writing it to prevent.
//
// An empty `globs` means all source, which is the opposite reading of an empty
// list from §2.4's `tests.globs`, where no glob recognises no test file. Both
// are in §2.6's and §2.4's tables in those words, and neither is the matcher's
// to decide.
func (r *Rule) Applies(path string) bool {
	if glob.MatchAny(r.Exempt, path) {
		return false
	}
	return len(r.Globs) == 0 || glob.MatchAny(r.Globs, path)
}
