package rule

import (
	"fmt"
	"strings"
)

// Injected returns the rules §2.6.1.4 puts into one axis role's prompt: every
// resolved rule of that axis carrying no `detect` block, in corpus order.
//
// It is Compile's other half. §2.6.1 splits a corpus in two on one field — a
// rule with a `detect` block is evaluated mechanically by cr, and a rule
// without one is given to the role as text — and the two halves are exhaustive
// and disjoint by construction here, so no rule can fall between them and be
// enforced by neither. Compile's comment says the same thing from its side.
//
// The axis is the selector because §2.6.1.4 names the role by it: §2.6's table
// gives every rule an axis, defaulting to `convention`, and §2.5's role carries
// one. A rule reaches the role that enforces its axis and no other, so a
// convention standard does not arrive in the correctness role's prompt as
// unexplained prose.
//
// Corpus order is kept rather than re-sorted. §2.6 item 1 already put the
// corpus in the order a reader can predict — per-repository first, then global,
// then the profile's own, ascending by id inside each — so a repository's own
// standard is the first thing the role reads, and §2.1.1's requirement that the
// same inputs give the same result holds without a second ordering rule here.
func Injected(corpus []Resolved, axis string) []Resolved {
	injected := make([]Resolved, 0, len(corpus))
	for at := range corpus {
		resolved := &corpus[at]
		if resolved.Rule.Detect == nil && resolved.Rule.Axis == axis {
			injected = append(injected, *resolved)
		}
	}
	return injected
}

// Injection renders one rule as the text §2.6.1.4 injects into the prompt.
//
// What it carries is the rule and nothing about the diff. §2.6.1.4 grades any
// record this produces normally per §6.2, and §6.2's `cited` row is bought by a
// citation cr resolved — either outside the record's own unit, or stamped
// `origin: rule` by §6.2.5 against a hit in `rule-stats.ndjson`. A detect-less
// rule produces no hit, so it can buy neither: there is no path and no line
// here to become a citation, and an injection that named one would be cr
// asserting a location it never matched. The role reads the standard and finds
// its own evidence, exactly as it does without a rule.
//
// The id is in the text because §2.6 item 3 requires every record produced by
// this rule to carry it, and the prompt is where the agent learns it. The
// rationale is there because §2.6 item 4 has the record able to quote it, so
// the author learns the standard and not only the violation. The class,
// severity and kind are the shape §2.6's table gives records from this rule;
// without them the agent invents a class, and §6.4.1's dedup key, §7.4.1's
// waiver key and §7.3.3's new-class report all read that field.
//
// The layer is named last. §2.6 item 1 resolves three of them and item 2 lets a
// higher one override a lower whole, so a role reading a standard it disagrees
// with needs to know which file to open — and the answer is the resolved path,
// not the id.
func (r *Resolved) Injection() string {
	return strings.Join([]string{
		fmt.Sprintf("rule %s (axis %s, class %s, severity %s, kind %s)",
			r.Rule.ID, r.Rule.Axis, r.Rule.Class, r.Rule.Severity, r.Rule.Kind),
		r.Rule.Title,
		r.Rule.Rationale,
		"from " + r.Path,
	}, "\n")
}
