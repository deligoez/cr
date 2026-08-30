package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/note"
)

// runIn runs one cr command against whatever CR_HOME already points at, and
// returns what it printed and what it refused.
//
// It takes the whole command line rather than prepending one, because these
// tests record with `cr note` and `cr answer` and then read with `cr context`:
// the store one command writes has to be the store the next one finds, so all
// three run against one state root.
func runIn(t *testing.T, args ...string) (printed string, err error) {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	// Executed first and read after: a return statement evaluates its
	// operands left to right, so reading the buffer in it would read what
	// the command printed before it ran.
	err = cmd.Execute()
	return out.String(), err
}

// runContext runs `cr context` with args.
func runContext(t *testing.T, args ...string) (printed string, err error) {
	t.Helper()
	return runIn(t, append([]string{"context"}, args...)...)
}

// §3.6.5 asks for the notes *with provenance*, and this is what that has to
// mean: everything the store holds about a note reaches the reader.
//
// The two notes between them carry every field §3.6.1 and §3.6.2 name — a plain
// note recorded from one pull request, and an answer recorded from another,
// which is the only note that carries a record. The printed document is then
// compared against the store itself rather than against a list written here, so
// a field added to a note later is a field this test requires to be printed,
// and the fields are named individually as well, so an equality between two
// empty lists cannot pass for one.
func TestContextPrintsEveryFieldTheStoreHolds(t *testing.T) {
	layout := briefedHome(t, "CR-7")

	_, err := runIn(t, "note", "CR-7", "the deadline moved to Friday", "--source", "chat", "--pr", "9")
	require.NoError(t, err)
	_, err = runIn(t, "answer", answeredPR, "f3", "the retry is deliberate",
		"--source", "thread", "--repo", answeredSlug)
	require.NoError(t, err)

	out, err := runContext(t, "CR-7")
	require.NoError(t, err)

	var printed struct {
		IssueKey string      `json:"issue_key"`
		Notes    []note.Note `json:"notes"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &printed))
	assert.Equal(t, "CR-7", printed.IssueKey)

	stored, err := note.Load(layout, "CR-7")
	require.NoError(t, err)
	require.Len(t, stored, 2)
	assert.Equal(t, stored, printed.Notes, "§3.6.5: the store holds more than the reader was given")

	assert.Equal(t, "CR-7#n1", printed.Notes[0].ID)
	assert.Equal(t, "the deadline moved to Friday", printed.Notes[0].Text)
	assert.Equal(t, note.SourceChat, printed.Notes[0].Source)
	assert.Equal(t, 9, printed.Notes[0].PR)
	assert.False(t, printed.Notes[0].RecordedAt.IsZero(), "§3.6.1 requires a timestamp")
	assert.Empty(t, printed.Notes[0].Record, "a note that answers no record names none")

	assert.Equal(t, "CR-7#n2", printed.Notes[1].ID)
	assert.Equal(t, "f3", printed.Notes[1].Record, "§3.6.2's answer names the record it was given to")
	assert.Equal(t, answeredPRNum, printed.Notes[1].PR)
}
