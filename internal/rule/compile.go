package rule

import (
	"fmt"
	"regexp"
)

// ModeRegex is the only value §2.6.1.2 admits for `detect.mode` in v0.1.
//
// It is a stated value rather than a defaulted row. §2.6's table gives `axis`,
// `severity` and `kind` a default in the table itself, and gives `detect.mode`
// none — §2.6.1.2 states the requirement instead, in the section that runs the
// pattern. So a block that leaves `mode` blank has not said how its pattern
// applies, and reading a default into it would be cr choosing a semantics for a
// detector its author never finished describing. v0.1 would guess right today
// and wrong the first time a second mode exists, against a file written before
// there was a second mode to mean.
const ModeRegex = "regex"

// Compile turns a resolved corpus into the matchers §2.6.1.1 evaluates,
// carrying over only the rules that have a `detect` block.
//
// A rule without one is not skipped so much as not addressed here: §2.6.1.4
// injects it into its axis role's prompt as text, which is a different
// mechanism with a different task behind it. What this function guarantees to
// the mechanism is narrower and worth stating: every Matcher it returns holds a
// pattern that compiled, so detection never has to decide what to do about one
// that did not.
//
// §2.6.1.2's abort is the whole of the error path. A `detect` block cr cannot
// run is not a rule cr can quietly leave out — its author wrote it to enforce a
// standard, and a corpus that dropped it would review the change against every
// rule but that one and report full coverage. So the first unusable block stops
// the command with the code §2.6.5 gives a malformed rule, naming the rule, the
// field, and the file it is written in.
func Compile(corpus []Resolved) ([]Matcher, error) {
	matchers := make([]Matcher, 0, len(corpus))
	for at := range corpus {
		resolved := &corpus[at]
		fix, err := resolved.compileFix()
		if err != nil {
			return nil, err
		}
		if resolved.Rule.Detect == nil {
			continue
		}
		pattern, err := resolved.compile()
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, Matcher{Rule: resolved.Rule, Pattern: pattern, Fix: fix})
	}
	return matchers, nil
}

// compiles holds one parsed rule to the checks Compile makes, at the moment the
// rule is loaded: its `fix.replace` compiles per §2.6.2.1, and its `detect`
// block, when it carries one, names `regex` and a pattern that compiles per
// §2.6.1.2. path is the file every fault is reported against.
//
// It runs in the loader because §2.6 item 5 aborts on a malformed rule file
// wherever one is read, not only where its detector runs. A command that loads
// the corpus without compiling it — `cr rules list`, `cr record`'s class check,
// the draft's provenance — would otherwise list a rule `cr rules check` refuses
// as effective and exit 0 over it. Compile keeps calling the same two methods,
// so the check has one spelling and a Resolved built by hand is still refused.
func compiles(path string, r *Rule) error {
	resolved := Resolved{Rule: *r, Path: path}
	if _, err := resolved.compileFix(); err != nil {
		return err
	}
	if r.Detect == nil {
		return nil
	}
	_, err := resolved.compile()
	return err
}

// compile holds one `detect` block to §2.6.1.2: `mode` is `regex`, and
// `pattern` is a regular expression in Go `regexp` syntax.
//
// The mode is read before the pattern. A block declaring a mode cr does not
// have is a block whose pattern cr has no way to read, so compiling it first
// would answer a question about a syntax the author was not writing in — and a
// pattern valid in one dialect and not another would then be reported as a
// broken regular expression rather than as an unsupported mode.
func (r *Resolved) compile() (*regexp.Regexp, error) {
	if r.Rule.Detect.Mode != ModeRegex {
		return nil, &MalformedError{
			File:  r.Path,
			Field: "detect.mode",
			Problem: fmt.Sprintf(
				"of rule %q is %q, and §2.6.1.2 admits only %q in v0.1",
				r.Rule.ID, r.Rule.Detect.Mode, ModeRegex,
			),
		}
	}
	pattern, err := regexp.Compile(r.Rule.Detect.Pattern)
	if err != nil {
		return nil, &MalformedError{
			File:  r.Path,
			Field: "detect.pattern",
			Problem: fmt.Sprintf(
				"of rule %q is not a Go regexp: %v",
				r.Rule.ID, err,
			),
		}
	}
	return pattern, nil
}
