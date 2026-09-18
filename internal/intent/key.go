package intent

import (
	"fmt"
	"regexp"
)

// keyPatternField is the dotted §2.7 spelling of the setting §3.2 makes
// overridable, carried on every error about it so the fix names a config key
// rather than a Go field.
const keyPatternField = "intent.key_pattern"

// KeyOrigin names which of §3.2's four sources yielded the key.
//
// It is not decoration. §3.2 orders the sources, so the same key can be right
// from one and wrong from another, and a run that cannot say where its key came
// from cannot be argued with. §4.5.3's unavailable intent axis is the other
// reader: KeyAbsent is the condition it turns on.
type KeyOrigin string

// The origins, in the order §3.2 consults them.
const (
	// KeyAbsent is the zero value: no source yielded a key.
	KeyAbsent KeyOrigin = ""
	// KeyFromFlag is the --issue <KEY> flag, §3.2 item 1.
	KeyFromFlag KeyOrigin = "flag"
	// KeyFromBranch is the PR branch name, §3.2 item 2.
	KeyFromBranch KeyOrigin = "branch"
	// KeyFromTitle is the PR title, §3.2 item 3.
	KeyFromTitle KeyOrigin = "title"
	// KeyFromBody is the PR body, §3.2 item 4.
	KeyFromBody KeyOrigin = "body"
	// KeyRecorded is a key read back out of §2.3's state rather than
	// resolved from a source. meta.json records the key and not where it
	// came from, so a round read off disk can say that a key resolved and
	// cannot say which of the four above yielded it. It is no source of
	// §3.2's, which is the point: code branching on which source won
	// cannot mistake one of these for a live resolution.
	KeyRecorded KeyOrigin = "recorded"
)

// KeySources are the four §3.2 sources, held in one value so their order is a
// property of this package rather than of each call site.
//
// They arrive as plain strings. The branch, title, and body are a pull
// request's, and fetching them is internal/gh's job — resolution is string work
// over text somebody else read, and reaching for GitHub from here would put a
// network call inside a pure function and give internal/intent a second reason
// to exist. It also keeps the ordering testable without a pull request.
type KeySources struct {
	// Flag is the --issue <KEY> value, empty when the flag was not given.
	Flag string
	// Branch is the PR's head branch name.
	Branch string
	// Title is the PR title.
	Title string
	// Body is the PR body.
	Body string
}

// Key is one resolution's outcome: the key and where it was found.
type Key struct {
	// Value is the matched text, empty when no source yielded one. §3.2's
	// fallback reads this: an empty Value is the empty intent it describes.
	Value string
	// Origin is the source Value came from, KeyAbsent when there is none.
	Origin KeyOrigin
}

// KeyPatternError reports an `intent.key_pattern` that will not compile.
//
// §3.2 makes the pattern user-overridable and says nothing about a bad one, but
// cr has already answered this question twice: §2.6.1.2 aborts with exit code 3
// naming the rule when a `detect.pattern` fails to compile, and a profile's
// `tests.count_pattern` follows it. A third answer here would make the same
// mistake cost something different depending on which setting carried it.
//
// It is a configuration failure and not a validation one: nothing was read, and
// no input data was judged. §11.2 codes it 3, and internal/cli maps it there.
type KeyPatternError struct {
	// Pattern is the expression as configured.
	Pattern string
	// Err is the compile failure regexp reported.
	Err error
}

func (e *KeyPatternError) Error() string {
	return fmt.Sprintf("%s %q: %v", keyPatternField, e.Pattern, e.Err)
}

// Unwrap exposes the compile failure, so a caller can read what regexp objected
// to rather than only that something did.
func (e *KeyPatternError) Unwrap() error { return e.Err }

// ResolveKey returns the issue key §3.2 resolves from the first source that
// yields a match, and the source it came from.
//
// The pattern is applied to all four sources alike, --issue included. §3.2
// makes the flag item 1 of one ordered list and gives it no separate rule, so a
// flag whose value the configured pattern does not recognise yields no match
// and the next source is consulted — the same fall-through every other source
// gets. That is a real consequence rather than an oversight: with a
// `key_pattern` narrowed to one tracker's shape, a hand-typed key of another
// shape does not silently become the key. §3.2's own fallback is why it is not
// an error either — every way of finding no key ends in an empty intent.
//
// Order is the whole of the contract, so a later source never wins over an
// earlier match, including when the earlier one is not the key a human would
// have picked. The pattern is what cr has; preferring the "better-looking"
// match further down would make the result depend on a judgement cr is not
// allowed to form.
//
// The match, not the source, is the key: §3.2's default is unanchored and has
// to be, because a branch is `feature/CR-123-slug` and a title is a sentence,
// and an anchored pattern would find a key in neither. The cost is that the
// leftmost match wins wherever it starts, so `XCR-123` reads as the key
// `XCR-123` rather than as `CR-123` inside a longer word — the expression is
// what decides, and a corpus needing word boundaries writes them into
// `intent.key_pattern`, which Go's regexp spells `\b`.
//
// A match is a non-empty one. An overriding pattern that can match the empty
// string, `[A-Z]*` being the plausible attempt at "any key", would otherwise
// match at position zero of the first source and report an empty key as found,
// which is worse than the no-key fallback it would have taken.
func ResolveKey(sources KeySources, pattern string) (Key, error) {
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return Key{}, &KeyPatternError{Pattern: pattern, Err: err}
	}
	ordered := []struct {
		origin KeyOrigin
		text   string
	}{
		{KeyFromFlag, sources.Flag},
		{KeyFromBranch, sources.Branch},
		{KeyFromTitle, sources.Title},
		{KeyFromBody, sources.Body},
	}
	for _, source := range ordered {
		if match := expression.FindString(source.text); match != "" {
			return Key{Value: match, Origin: source.origin}, nil
		}
	}
	return Key{}, nil
}

// PatternAdmits reports whether naming key on the command line would resolve
// key, under `intent.key_pattern` as it now reads.
//
// It is ResolveKey's own answer rather than a second one: the question a caller
// asks here is exactly "would `--issue <key>` yield <key>", and the way to be
// sure of that is to run the flag through the same resolution, with the same
// compiled pattern and the same non-empty-match rule. A separate MatchString
// would drift from it, and would answer yes for a pattern that matches only a
// fragment of the key — `[0-9]+` finds `7` inside `CR-7`, which resolves a key
// that is not the one asked about.
//
// An empty key is not admitted. Nothing resolves it, and §3.2's fallback is
// what an absent key already means.
//
// A pattern that will not compile is not admitted either. ResolveKey aborts on
// it with KeyPatternError long before any caller of this function is reached,
// so the only question left here is which of two messages to print, and the
// honest answer for a pattern that cannot run is that it recognises nothing.
func PatternAdmits(pattern, key string) bool {
	if key == "" {
		return false
	}
	resolved, err := ResolveKey(KeySources{Flag: key}, pattern)
	if err != nil {
		return false
	}
	return resolved.Value == key
}
