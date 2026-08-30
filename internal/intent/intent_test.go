package intent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
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
