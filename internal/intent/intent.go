package intent

import (
	"errors"
	"strconv"

	"github.com/deligoez/cr/internal/axis"
)

// Unavailable is one entry of §4.5.4's report at the granularity §4.5.3 works
// at: a whole axis that did not run, and the reason it could not.
//
// It is the shape testadequacy.Unavailable already holds — two fields, text
// derived from them, finding.HonestyDisclosure satisfied — with `Lens` spelled
// `Axis`. That is not a cosmetic difference. §4.5.4 lists "a disabled axis, an
// unavailable axis, the unavailable reinvention half of §4.3.1" as three
// different things, and profile.MissingProfile keeps the same distinction by
// prefixing one list with `axis` and the other with `lens`. §4.5.3 takes out the
// whole intent axis, so an entry calling it a lens would understate what did not
// look.
//
// The two shapes are not unified into one struct because finding.HonestyDisclosure
// already is the union: §4.5.4's report is a list of things that must be
// printed, and the interface is what makes a list of them collectable without
// every reason in the spec having to fit one set of field names.
type Unavailable struct {
	// Axis is the axis id of §1.5 that did not run.
	Axis string `json:"axis"`
	// Reason is why it could not, and what would make it run.
	Reason string `json:"reason"`
}

// Disclosure satisfies finding.HonestyDisclosure, so the entry reaches the
// reader through the one channel §11.1 exempts from `--quiet` rather than
// through a message a flag can silence. The text is derived from the two fields,
// so what is printed and what a caller reads as data cannot drift apart.
func (u Unavailable) Disclosure() string {
	return "axis " + u.Axis + " unavailable, per §4.5.3: " + u.Reason
}

// AuthorDisclosure is the entry as §8.4.3's review body words it for the pull
// request's author: the axis did not run and what cr therefore did not check,
// with no flag to pass and no section to read. Reason is worded for the
// reviewer who can act on it.
func (u Unavailable) AuthorDisclosure() string {
	return "axis " + u.Axis + " did not run: cr found no issue linked to this pull request, " +
		"so it did not check the change against what an issue asks for"
}

// Intent is one run's issue text together with the key it was read for.
//
// The empty value is §3.2's empty intent: no key, no text. It is a state the
// review runs in rather than a failure, and Unavailability is how the rest of cr
// finds out it is in it.
type Intent struct {
	// Key is §3.2's resolution, with Origin KeyAbsent when no source
	// yielded one.
	Key Key `json:"key"`
	// Text is the issue text read for Key, empty when there is no key.
	Text string `json:"text"`
	// Pattern is the `intent.key_pattern` the resolution ran with, kept so
	// §4.5.4's reason can name what was searched for. A key the user can
	// see in the branch name and a pattern that does not match its shape
	// look identical from the outside, and the pattern is the half cr knows.
	Pattern string `json:"pattern"`
	// read is the Reading Text came from, nil when no text was read. It is
	// held by pointer so an Intent, passed by value across cr, stays small.
	read *Reading
}

// Reading is the read this intent's text came from, which is what the §3.3
// checks over it take.
func (i Intent) Reading() Reading {
	if i.read == nil {
		return Reading{Text: i.Text}
	}
	return *i.read
}

// Unavailability returns §4.5.4's entry for the intent axis, and true, when
// §4.5.3 applies; it returns false when a key resolved.
//
// It mirrors profile.Selection.Missing — a report plus a bool, derived rather
// than stored — so the axis's state cannot disagree with the resolution it came
// from. There is no Intent that carries issue text and reports itself
// unavailable, and none that resolved no key and reports itself available.
//
// The bool is also §4.6.6's condition. When it is true there is no intent role
// and no mapping to produce, `cr map record` is neither required nor accepted,
// the mapping is empty, and §4.1.2 and §4.1.3 raise nothing: an absent tracker
// is not an unmapped unit. Acting on that is the fan-out's; supplying the one
// fact it turns on is this.
func (i Intent) Unavailability() (Unavailable, bool) {
	if i.Key.Origin != KeyAbsent {
		return Unavailable{}, false
	}
	if i.Pattern == GitHubKeyPattern {
		return Unavailable{
			Axis: axis.Intent,
			Reason: "GitHub links this pull request to no issue it closes, and a " + TrackerGitHub +
				" tracker reads no key out of the branch, the title or the body; pass --issue <number>, " +
				"or link the issue with a closing keyword such as `Closes #12`",
		}, true
	}
	return Unavailable{
		Axis: axis.Intent,
		Reason: "no issue key matched " + keyPatternField + " " + strconv.Quote(i.Pattern) +
			" in the --issue flag, the branch name, the title, or the body; " +
			"pass --issue <KEY>, or set " + keyPatternField +
			" to the shape this tracker's keys have",
	}, true
}

// Recorded is a round's intent as §2.3's meta.json carries it: the key §3.2
// resolved when the round was opened, and the `intent.key_pattern` in force.
//
// It exists so a command reading a recorded round can answer §4.5.3 — the
// intent axis is unavailable when no issue key resolves — without re-running
// §3.2. §10.1.3 is the caller: `cr status` reports the axes of the round that
// was opened, and a fresh resolution would report the axes of a round nobody
// opened, since a branch can be renamed and a title edited after the fact.
//
// Nothing is invented. An empty key is §3.2's absent one, which is exactly what
// meta.json carries for a round that resolved none, and a key that was recorded
// is marked KeyRecorded rather than attributed to a source this cannot know.
func Recorded(key, pattern string) Intent {
	if key == "" {
		return Intent{Pattern: pattern}
	}
	return Intent{Key: Key{Value: key, Origin: KeyRecorded}, Pattern: pattern}
}

// Resolve carries out §3.2 and returns the round's intent: the key resolved from
// the first source that yields a match, and the issue text read for it, or
// §3.2's empty intent when no source yields one.
//
// The fallback is why this exists instead of a call site pairing ResolveKey with
// Read. "Continue with an empty intent" fails in a way that does not look like a
// failure: a source consulted for an empty key runs the tracker command with
// `{key}` substituted to nothing, and what comes back is whatever that tracker
// does with a blank issue id — an error on one, some default issue on another.
// Either way the no-key case would stop being one answer. Here the source is
// never reached without a key, so continuing costs nothing and touches no
// network.
//
// That holds for §3.1.4's file too, and it is the consequence worth stating.
// `--intent-file` bypasses the command, not the key: §3.2's fallback is written
// without exceptions, and §3.3 forms every claim id as `<ISSUE-KEY>#c<n>`, so
// text read for no key would be text no claim could be extracted from. A file
// supplies the issue text for a key that resolved; it does not supply the key.
func Resolve(sources KeySources, pattern string, source Source) (Intent, error) {
	key, err := ResolveKey(sources, pattern)
	if err != nil {
		return Intent{}, err
	}
	if key.Origin == KeyAbsent {
		return Intent{Pattern: pattern}, nil
	}
	reading, err := Read(source, key.Value)
	if refused, ok := errors.AsType[*CommandError](err); ok {
		refused.Resolved = true
	}
	if err != nil {
		return Intent{}, err
	}
	return Intent{Key: key, Text: reading.Text, Pattern: pattern, read: &reading}, nil
}
