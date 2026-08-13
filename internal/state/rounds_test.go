package state

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The rounds/<n>/ part of the §2.3 table is created on demand: a state
// directory has no round in it until a round is opened, and the round that is
// opened holds every artefact the table names.
func TestARoundDirectoryHoldsEveryArtefact(t *testing.T) {
	l := lockedPR(t)

	_, err := os.Stat(l.RoundDir("acme", "web", 42, 1))
	assert.ErrorIs(t, err, os.ErrNotExist,
		"EnsurePR opens no round: §9.3.3 does, and meta.json starts at 0")

	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, held.EnsureRound(1))
	require.NoError(t, held.Unlock())

	assert.Equal(t, []string{"draft.md", "rendered.json", "posted.json", "summary.json"},
		RoundFiles(), "the artefact set is the §2.3 table, in table order")

	for _, name := range RoundFiles() {
		info, err := os.Stat(l.RoundFile("acme", "web", 42, 1, name))
		require.NoError(t, err, name)
		assert.False(t, info.IsDir(), name)
	}

	draft, err := l.ReadRound("acme", "web", 42, 1, FileDraft)
	require.NoError(t, err)
	assert.Empty(t, string(draft), "an unrendered draft is an empty Markdown file")

	for _, name := range []string{FileRendered, FilePosted, FileSummary} {
		body, err := l.ReadRound("acme", "web", 42, 1, name)
		require.NoError(t, err, name)
		assert.Equal(t, emptyDocument, string(body),
			"a JSON artefact starts as a document with nothing in it, not as an unparseable empty file")
	}
}

// §9.3.5: earlier rounds are history, and a command that writes the current
// round MUST leave them intact. Opening round 2 and filling every one of its
// artefacts must therefore leave round 1's four files byte-identical.
func TestASecondRoundLeavesTheFirstIntact(t *testing.T) {
	l := lockedPR(t)

	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	for _, name := range RoundFiles() {
		require.NoError(t, held.WriteRound(1, name, []byte("round one "+name)))
	}
	require.NoError(t, held.Unlock())

	first := make(map[string][]byte, len(RoundFiles()))
	for _, name := range RoundFiles() {
		body, err := l.ReadRound("acme", "web", 42, 1, name)
		require.NoError(t, err, name)
		first[name] = body
	}

	held, err = l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	for _, name := range RoundFiles() {
		require.NoError(t, held.WriteRound(2, name, []byte("round two "+name)))
	}
	require.NoError(t, held.Unlock())

	for _, name := range RoundFiles() {
		body, err := l.ReadRound("acme", "web", 42, 1, name)
		require.NoError(t, err, name)
		assert.Equal(t, string(first[name]), string(body),
			"round 2 disturbed round 1's "+name)

		body, err = l.ReadRound("acme", "web", 42, 2, name)
		require.NoError(t, err, name)
		assert.Equal(t, "round two "+name, string(body))
	}
}
