package note

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// storeRoot returns a layout with the global tree already created.
func storeRoot(t *testing.T) state.Layout {
	t.Helper()
	l := state.New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, l.Init())
	return l
}

// stored decodes the notes on disk for one issue key, which is what a later
// round and `cr context` will read rather than the value Append returned.
func stored(t *testing.T, l state.Layout, issueKey string) []Note {
	t.Helper()
	body, err := os.ReadFile(l.ContextFile(issueKey))
	require.NoError(t, err)

	notes := make([]Note, 0)
	for _, line := range strings.Split(strings.TrimRight(string(body), "\n"), "\n") {
		if line == "" {
			continue
		}
		var n Note
		require.NoError(t, json.Unmarshal([]byte(line), &n))
		notes = append(notes, n)
	}
	return notes
}

// §3.6.1: a note carries an id of the form `<ISSUE-KEY>#n<n>`, a timestamp, the
// pull request it came from, and the source. The file is read back rather than
// the return value trusted, because it is the file every later round loads.
func TestAppendRecordsTheFieldsTheSpecNames(t *testing.T) {
	l := storeRoot(t)
	at := time.Date(2026, 8, 30, 9, 15, 0, 0, time.FixedZone("UTC+3", 3*60*60))

	first, err := Append(l, "CR-1", "the deadline moved to Friday", SourceChat, 42, at)
	require.NoError(t, err)
	assert.Equal(t, "CR-1#n1", first.ID)
	assert.Equal(t, "the deadline moved to Friday", first.Text)
	assert.Equal(t, SourceChat, first.Source)
	assert.Equal(t, 42, first.PR)
	assert.True(t, at.Equal(first.RecordedAt), "the timestamp is the instant it was given")
	assert.Equal(t, time.UTC, first.RecordedAt.Location(),
		"stored in UTC, so two machines order two notes the same way")

	second, err := Append(l, "CR-1", "the API team owns the retry", SourceMeeting, 42, at)
	require.NoError(t, err)
	assert.Equal(t, "CR-1#n2", second.ID, "§3.6.1 numbers over the notes already stored")

	assert.Equal(t, []Note{first, second}, stored(t, l, "CR-1"),
		"the store holds both notes, in the order they were appended")
}

