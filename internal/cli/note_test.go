package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// runNote runs `cr note` with args against a fresh state root and returns the
// root, whatever it printed, and whatever it refused.
func runNote(t *testing.T, args ...string) (root, printed string, err error) {
	t.Helper()
	root = filepath.Join(t.TempDir(), ".cr")
	t.Setenv(state.HomeEnv, root)

	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"note"}, args...))
	// Executed first and read after: a return statement evaluates its
	// operands left to right, so reading the buffer in it would read what
	// the command printed before it ran.
	err = cmd.Execute()
	return root, out.String(), err
}

// §3.6.1 through the command: the note lands in §2.2's context store, keyed by
// the issue and not by the pull request, carrying the id, the timestamp, the
// pull request, and the source.
func TestNoteCommandRecordsAgainstTheIssueKey(t *testing.T) {
	root, out, err := runNote(t,
		"CR-1", "the deadline moved to Friday", "--source", "chat", "--pr", "42")
	require.NoError(t, err)

	var printed struct {
		Note note.Note `json:"note"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &printed))
	assert.Equal(t, "CR-1#n1", printed.Note.ID)
	assert.Equal(t, note.SourceChat, printed.Note.Source)
	assert.Equal(t, 42, printed.Note.PR)
	assert.False(t, printed.Note.RecordedAt.IsZero(), "§3.6.1 requires a timestamp")

	stored, err := os.ReadFile(state.New(root).ContextFile("CR-1"))
	require.NoError(t, err)
	assert.Contains(t, string(stored), "the deadline moved to Friday")
	assert.Contains(t, string(stored), `"id":"CR-1#n1"`)
}

// §12.1's other shape for this command. A terminal reader gets the id and the
// source, not the document: §3.6.6 makes a note revocable by id, so the id is
// the one thing the run produced that the user does not already have, and
// §8.1.6 will disclose the source in every posted body resting on the note.
func TestATerminalNoteNamesTheIdAndTheSource(t *testing.T) {
	crHome(t)

	out := throughATerminal(t, "note", "CR-1", "the deadline moved", "--source", "chat", "--pr", "42")

	assert.Contains(t, out, "recorded ")
	assert.Contains(t, out, "\x1b[36mCR-1#n1\x1b[0m", "the id is accented, as every terminal rendering accents its answer")
	assert.Contains(t, out, " from chat")
}

// The round-11 finding unspecified-flag-requiredness: §3.6.1 shows `--source`
// in its synopsis and never says it is required, so cr says it. An absent value
// and one outside §3.6.3's set are both the invocation being wrong rather than
// data in a file being wrong, which §11.2 codes 2 and not 1 — and an unmapped
// error is already exactly that in exitCodeFor, so neither needs a mapping of
// its own. `--pr` goes the same way: §3.6.1 has the note record the pull
// request it came from, and cr forms no opinion about which one that was.
func TestNoteRefusesAnAbsentOrUnlistedSourceWithTheUsageCode(t *testing.T) {
	for name, args := range map[string][]string{
		"no source at all":  {"CR-1", "hearsay", "--pr", "42"},
		"an empty source":   {"CR-1", "hearsay", "--source", "", "--pr", "42"},
		"an unlisted value": {"CR-1", "hearsay", "--source", "gossip", "--pr", "42"},
		"no pull request":   {"CR-1", "hearsay", "--source", "chat"},
		"no text":           {"CR-1", "--source", "chat", "--pr", "42"},
		"no issue key":      {"--source", "chat", "--pr", "42"},
	} {
		t.Run(name, func(t *testing.T) {
			root, out, err := runNote(t, args...)
			require.Error(t, err)
			assert.Equal(t, ExitUsage, exitCodeFor(err),
				"§11.2 codes a malformed invocation 2")
			assert.Empty(t, out, "a refused run prints no note")
			assert.NoFileExists(t, state.New(root).ContextFile("CR-1"))
		})
	}
}
