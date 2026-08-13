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

// meta.json carries round 0 until §9.3.3 opens round 1, so 0 is the signal that
// no round has been opened rather than a round of its own. Asking for its
// directory must be refused on every route in and leave nothing on disk, so a
// pull request never acquires a history it does not have.
func TestRoundZeroIsNotARound(t *testing.T) {
	l := lockedPR(t)

	meta, err := l.ReadMeta("acme", "web", 42)
	require.NoError(t, err)
	require.Equal(t, 0, meta.Round, "a state directory starts with no round opened")

	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	for name, err := range map[string]error{
		"ensure": held.EnsureRound(meta.Round),
		"write":  held.WriteRound(meta.Round, FileDraft, []byte("drafted")),
	} {
		assert.ErrorContains(t, err, "no round has been opened", name)
	}
	require.NoError(t, held.Unlock())

	_, err = l.ReadRound("acme", "web", 42, meta.Round, FileDraft)
	assert.ErrorContains(t, err, "no round has been opened")

	_, err = os.Stat(l.RoundDir("acme", "web", 42, meta.Round))
	assert.ErrorIs(t, err, os.ErrNotExist, "rounds/0/ must never be created")

	held, err = l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	assert.NoError(t, held.EnsureRound(1), "1 is the first round §9.3.3 opens")
	require.NoError(t, held.Unlock())
}

// The §2.3 table decides what a round directory holds, so a name it does not
// give a round is refused rather than written beside the four artefacts.
func TestOnlyTheTablesArtefactsReachARoundDirectory(t *testing.T) {
	l := lockedPR(t)

	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, held.EnsureRound(1))
	assert.ErrorContains(t, held.WriteRound(1, "notes.md", []byte("scratch")),
		"§2.3 gives a round no such artefact")
	assert.ErrorContains(t,
		UpdateRoundJSON(held, 1, "notes.json", func(*map[string]int) {}),
		"§2.3 gives a round no such artefact")
	require.NoError(t, held.Unlock())

	entries, err := os.ReadDir(l.RoundDir("acme", "web", 42, 1))
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.ElementsMatch(t, RoundFiles(), names)
}
