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

