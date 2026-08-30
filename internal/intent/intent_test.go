package intent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
)

// §3.2's fallback is two claims in one sentence — cr continues, and the intent
// axis is marked unavailable — and a review that stops instead satisfies
// neither. Every source here is populated and none carries a key, which is the
// realistic shape of it: a repository whose work is tracked somewhere cr was
// never told about.
//
// The tracker script writes a marker file, so "continues with an empty intent"
// is asserted as a command that never ran rather than only as an empty string.
// A run that consults the source anyway would substitute nothing for the key and
// come back with whatever that tracker does with a blank issue id — an error on
// one, a default issue on another — and the no-key case would stop having one
// answer.
func TestNoIssueKeyContinuesWithAnEmptyIntent(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	tracker := stubTracker(t, "touch "+marker)

	intent, err := Resolve(KeySources{
		Branch: "feature/add-a-thing",
		Title:  "add a thing",
		Body:   "There is no tracker for this one.",
	}, specDefaultPattern, Source{Cmd: []string{tracker, "issue", "view", Placeholder}})

	require.NoError(t, err, "§3.2 continues; finding no key is not a refusal")
	assert.Equal(t, Key{}, intent.Key)
	assert.Empty(t, intent.Text, "§3.2's empty intent")
	assert.NoFileExists(t, marker, "no key means no issue to read, so the tracker is never started")

	unavailable, marked := intent.Unavailability()
	require.True(t, marked, "§4.5.3 marks the intent axis unavailable when no key resolves")
	assert.Equal(t, axis.Intent, unavailable.Axis, "the whole axis is out, not a half of one")
	assert.NotEmpty(t, unavailable.Reason, "§4.5.4 requires the reason, not only the fact")
}

// §3.1.4's file is the one place the fallback looks avoidable: the issue text is
// sitting on disk, so reading it and carrying on looks like a kindness. It is
// not one. §3.2's fallback is written without exceptions, and §3.3 forms every
// claim id as `<ISSUE-KEY>#c<n>`, so text held for no key is text no claim can
// be extracted from — the axis would report itself available and then fill no
// cell. `--intent-file` bypasses the command, not the key.
//
// The file's contents would be unmistakable in Text, so a run that read it
// anyway cannot pass this quietly.
func TestAnIntentFileDoesNotStandInForAMissingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(path, []byte("Add a thing to the thing.\n"), 0o600))

	intent, err := Resolve(KeySources{Branch: "feature/add-a-thing"}, specDefaultPattern, Source{File: path})

	require.NoError(t, err)
	assert.Empty(t, intent.Text, "the file supplies the text for a key, never the key")

	_, marked := intent.Unavailability()
	assert.True(t, marked, "§4.5.3 turns on the key, and no tracker access was needed to find there is none")
}

// The fallback is only worth anything if the ordinary path is untouched, and
// this is the half that a condition inverted anywhere in Resolve or
// Unavailability would break silently: a review reporting the intent axis
// unavailable on a perfectly well-keyed pull request would drop §4.1 entirely
// and still come back complete, because §4.5.4 would have disclosed it and
// §4.6.6 would have excused every cell it did not fill.
func TestAResolvedKeyLeavesTheIntentAxisAvailable(t *testing.T) {
	tracker := stubTracker(t, `printf 'Add a thing to the thing.\n'`)

	intent, err := Resolve(
		KeySources{Branch: "feature/CR-123-add-a-thing"},
		specDefaultPattern,
		Source{Cmd: []string{tracker, "issue", "view", Placeholder}},
	)

	require.NoError(t, err)
	assert.Equal(t, Key{Value: "CR-123", Origin: KeyFromBranch}, intent.Key)
	assert.Equal(t, "Add a thing to the thing.\n", intent.Text, "the issue text is read for the key that resolved")

	unavailable, marked := intent.Unavailability()
	assert.False(t, marked, "the axis ran, so §4.5.4 has nothing to report about it")
	assert.Equal(t, Unavailable{}, unavailable, "and no entry to hand a caller that ignores the bool")
}

// §11.1 exempts every lens of §4.5.4 that did not run from `--quiet`, and the
// exemption belongs to the shared writer rather than to each call site, so the
// report has to arrive there as a disclosure. Implementing
// finding.HonestyDisclosure is what makes that possible before the writer exists
// — the same contract profile.MissingProfile and testadequacy.Unavailable
// already satisfy, so §4.5.4 collects all three through one interface.
//
// The text is asserted against the fields rather than against a literal, because
// the two must not be able to drift: an axis counted as out in the data and left
// out of the printed report would be honest to a caller reading JSON and silent
// to the human reading a terminal.
//
// An empty Source is the second claim here. §3.1.1 refuses an empty `intent.cmd`
// before anything is started, so a nil error proves the source was not merely
// unused but never reached.
func TestTheUnavailableIntentAxisIsAnHonestyDisclosure(t *testing.T) {
	var _ finding.HonestyDisclosure = Unavailable{}

	intent, err := Resolve(KeySources{Branch: "feature/add-a-thing"}, specDefaultPattern, Source{})
	require.NoError(t, err)
	unavailable, marked := intent.Unavailability()
	require.True(t, marked)

	disclosure := unavailable.Disclosure()
	assert.Contains(t, disclosure, unavailable.Axis)
	assert.Contains(t, disclosure, unavailable.Reason)
	// §4.5 spends `disabled` and `unavailable` on two different states, and
	// this is the second: nothing was switched off, a prerequisite the axis
	// cannot supply itself is missing.
	assert.Contains(t, disclosure, "axis "+axis.Intent+" unavailable")
	// A key the author can see in the branch and a pattern that does not
	// match its shape look identical from outside, so the reason names the
	// setting and the expression the resolution actually ran with.
	assert.Contains(t, disclosure, "intent.key_pattern")
	assert.Contains(t, disclosure, specDefaultPattern)
	// §12.4's next actionable step, which is the user's: the flag §3.2 puts
	// first, or the pattern that would have recognised what is already there.
	assert.Contains(t, disclosure, "--issue")
}

// §3.2's fallback is for finding no key, and nothing else. Two failures pass
// through the same function and neither is one: a `intent.key_pattern` that will
// not compile searched nothing, and a tracker that refused was asked about a key
// that does exist. Swallowing either into the empty intent would turn a
// configuration fault the user can fix into an intent axis silently marked
// unavailable — §4.5.4 would disclose it, honestly and about the wrong thing,
// and the round would carry on with no claims.
func TestResolveSurfacesTheFailuresOfTheStepsItComposes(t *testing.T) {
	_, err := Resolve(KeySources{Branch: "feature/CR-1-add-a-thing"}, `[A-Z`, Source{})
	var pattern *KeyPatternError
	require.ErrorAs(t, err, &pattern, "an uncompilable pattern is a configuration fault, not a missing key")

	refusing := stubTracker(t, `echo 'no such issue' >&2; exit 1`)
	_, err = Resolve(
		KeySources{Branch: "feature/CR-1-add-a-thing"},
		specDefaultPattern,
		Source{Cmd: []string{refusing, "issue", "view", Placeholder}},
	)
	var command *CommandError
	require.ErrorAs(t, err, &command, "§3.1.3 fails on a non-zero exit; the key was found and the issue was not")
	assert.Contains(t, command.Stderr, "no such issue")
}
